/*
 * Chouf
 * author: Nicolas CARPi
 * copyright: 2022
 * license: MIT
 * repo: https://github.com/deltablot/chouf
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

// this will be overwritten during docker build
var choufVersion string = "dev"

type Result struct {
	Domain    string `json:"domain"`
	Ok        bool   `json:"ok"`
	LastCheck string `json:"last_check"`
}

type Website struct {
	Domain         string `yaml:"domain"`
	Name           string `yaml:"name,omitempty"`
	CertExpiration bool   `yaml:"cert_expiration,omitempty"`
	Endpoint       string `yaml:"endpoint,omitempty"`
	Status         int    `yaml:"status,omitempty"`
}

type Defaults struct {
	Endpoint string `yaml:"endpoint,omitempty"`
	Status   int    `yaml:"status,omitempty"`
}

type GeneralConfig struct {
	Port    int           `yaml:"port,omitempty"`
	Tick    time.Duration `yaml:"tick,omitempty"`
	Workers int           `yaml:"workers,omitempty"`
}

type Yaml struct {
	GeneralConfig GeneralConfig `yaml:"config,omitempty"`
	Defaults      Defaults      `yaml:"defaults,omitempty"`
	Inventory     []Website     `yaml:"inventory,flow"`
}

// global object holding config and results
type App struct {
	configPath    string
	debug         bool
	endpoint      string
	inventory     []Website
	port          int
	results       sync.Map
	smtp_from     string
	smtp_hostname string
	smtp_login    string
	smtp_password string
	smtp_port     int
	smtp_to       string
	status        int
	template      *template.Template
	tick          time.Duration
	workers       int
	ws            *websocket.Conn
}

func (app *App) init() {

	// adjust app with values from yml
	yml, err := readConfigFile(app.configPath)
	if err != nil {
		// if we cannot read the config file we simply exit
		log.Fatal("chouf: fatal: ", err)
	}
	if yml.GeneralConfig.Port != 0 {
		app.port = yml.GeneralConfig.Port
	}
	if yml.GeneralConfig.Tick != 0 {
		app.tick = yml.GeneralConfig.Tick
	}
	if yml.GeneralConfig.Workers != 0 {
		app.workers = yml.GeneralConfig.Workers
	}
	if len(yml.Defaults.Endpoint) > 0 {
		app.endpoint = yml.Defaults.Endpoint
	}
	if yml.Defaults.Status != 0 {
		app.status = yml.Defaults.Status
	}
	app.inventory = yml.Inventory

	// parse again as env and command line take over yml config
	flag.Parse()
	Debug("initialization done")
}

// global variables
var app App
var upgrader = websocket.Upgrader{}

func main() {
	// defaults
	const (
		defaultConfigPath   string = "/etc/chouf/config.yml"
		defaultEndpoint     string = "/"
		defaultPort         int    = 3003
		defaultStatus       int    = 200
		defaultTick                = 5 * time.Minute
		defaultWorkers      int    = 5
		defaultSmtpFrom            = "chouf@example.com"
		defaultSmtpHostname        = "mail.example.com"
		defaultSmtpLogin           = "chouf"
		defaultSmtpPassword        = "s3cret"
		defaultSmtpPort            = 587
		defaultSmtpTo              = "null@example.com"
	)
	var (
		wantVersion bool
		wantTest    bool
	)

	// command line options
	// CONFIG
	flag.StringVar(&app.configPath, "c", GetStrEnv("CHOUF_CONFIG", defaultConfigPath), "path to config file")
	flag.BoolVar(&app.debug, "d", GetBoolEnv("CHOUF_DEBUG"), "activate debug mode")
	flag.StringVar(&app.endpoint, "e", GetStrEnv("CHOUF_DEFAULT_ENDPOINT", defaultEndpoint), "endpoint to query")
	flag.IntVar(&app.port, "p", GetIntEnv("CHOUF_PORT", defaultPort), "server listening port")
	flag.IntVar(&app.status, "s", GetIntEnv("CHOUF_DEFAULT_STATUS", defaultStatus), "default http status code to expect for a valid response")
	flag.BoolVar(&wantTest, "t", false, "check config syntax and exit")
	flag.DurationVar(&app.tick, "T", GetDurationEnv("CHOUF_INTERVAL", defaultTick), "interval in minutes between checks")
	flag.BoolVar(&wantVersion, "v", false, "show program version and exit")
	flag.IntVar(&app.workers, "w", GetIntEnv("CHOUF_WORKERS", defaultWorkers), "number of workers")

	// TEMPLATE
	app.template = template.Must(template.ParseFiles("src/tmpl/index.tmpl"))

	// SMTP
	// only configured through ENV
	app.smtp_from = GetStrEnv("CHOUF_SMTP_FROM", defaultSmtpFrom)
	app.smtp_hostname = GetStrEnv("CHOUF_SMTP_HOSTNAME", defaultSmtpHostname)
	app.smtp_login = GetStrEnv("CHOUF_SMTP_LOGIN", defaultSmtpLogin)
	app.smtp_password = GetStrEnv("CHOUF_SMTP_PASSWORD", defaultSmtpPassword)
	app.smtp_port = GetIntEnv("CHOUF_SMTP_PORT", defaultSmtpPort)
	app.smtp_to = GetStrEnv("CHOUF_SMTP_TO", defaultSmtpTo)

	// parse it now to get the config path and version flag
	// we reparse later to override config values with env or command line
	flag.Parse()

	// happens early
	if wantVersion {
		fmt.Println("Chouf version:", choufVersion)
		os.Exit(0)
	}
	if wantTest {
		fmt.Println("Configuration syntax OK")
		os.Exit(0)
	}

	// load everything in app
	app.init()

	// create a ticker
	ticker := time.NewTicker(app.tick)
	defer ticker.Stop()
	hasTicked := false

	// create a wrapper of the ticker that ticks the first time immediately
	// from: https://stackoverflow.com/a/65300777
	tickerChan := func() <-chan time.Time {
		if !hasTicked {
			hasTicked = true
			c := make(chan time.Time, 1)
			c <- time.Now()
			return c
		}
		return ticker.C
	}

	// create a channel to receive signals from OS
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// channel to receive jobs
	jobs := make(chan Website, len(app.inventory))
	// channel to receive result for a website
	resChan := make(chan Result, len(app.inventory))

	// start workers
	for w := 1; w <= app.workers; w++ {
		go worker(w, jobs, resChan)
	}

	// adjust clock to local time
	time.LoadLocation("Local")

	// start webserver
	go func() {
		http.HandleFunc("/", getHtml)
		http.HandleFunc("/json", getJson)
		http.HandleFunc("/ws", getWs)
		http.HandleFunc("/favicon.ico", getFavicon)
		http.HandleFunc("/js", getJs)
		Debug(fmt.Sprint("Starting server on port: ", app.port))
		err := http.ListenAndServe(fmt.Sprint(":", app.port), nil)
		if err != nil {
			log.Println(fmt.Sprintf("chouf: error: could not start server on port %d! Skipping...", app.port))
		}
	}()

	log.Println("chouf: info: service started")
	log.Printf("chouf: info: %d sites in configuration", len(app.inventory))
	log.Printf("chouf: info: interval is set to %s", app.tick)
	log.Printf("chouf: info: server listening on port %d", app.port)

loop:
	for {
		// loop over channels
		select {
		// handle tick
		case <-tickerChan():
			// send websites from inventory on the jobs channel
			for _, website := range app.inventory {
				jobs <- website
			}
		case result := <-resChan:
			// when a new result arrives, write it to the websocket
			if app.ws != nil { // make sure there is a websocket connection
				if err := app.ws.WriteJSON(result); err != nil {
					log.Println("chouf: notice: could not write to websocket: closing it")
					app.ws.Close()
					app.ws = nil
				}
			}
			// loop the existing results and send an email if there is a change of state
			for _, site := range getResults() {
				if result.Domain == site.Domain && result.Ok != site.Ok {
					Debug("sending email")
					sendEmail(result)
				}
			}
		// handle OS signals
		case s := <-signalChan:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("chouf: info: received signal to exit. Adieu!")
				break loop
			case syscall.SIGHUP:
				app.init()
				log.Println("chouf: info: config reloaded")
			}
		}
	}
}

func sendEmail(result Result) error {
	state := "DOWN"
	body := "The website " + result.Domain + " appears to be down. We will let you know when it is up again.\r\n"
	if result.Ok {
		state = "UP"
		body = "The website " + result.Domain + " is responding correctly again. =)\r\n"
	}
	msg := []byte("To: " + app.smtp_to + "\r\n" +
		"Subject: [Chouf Alert] " + result.Domain + " is " + state + "!\r\n" +
		"\r\n" + body)

	hostname := app.smtp_hostname
	auth := smtp.PlainAuth("", app.smtp_login, app.smtp_password, hostname)
	return smtp.SendMail(fmt.Sprintf("%s:%d", hostname, app.smtp_port), auth, app.smtp_from, []string{app.smtp_to}, msg)
}

func worker(id int, jobs <-chan Website, resChan chan Result) {
	Debug(fmt.Sprintf("worker %d ready to receive work", id))
	for job := range jobs {
		Debug(fmt.Sprintf("worker %d now processing: %s", id, job.Domain))
		result := process(job)
		app.results.Store(result.Domain, result)
		resChan <- result
	}
}
func getHtml(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	data := struct {
		Version  string
		PageTime string
		Results  []Result
	}{
		choufVersion,
		time.Now().Format(time.RFC3339),
		getResults(),
	}

	err := app.template.ExecuteTemplate(w, "index", data)
	if err != nil {
		log.Print("chouf: error during template execution: %s", err)
	}
}

func getResults() []Result {
	// loop over the sync map
	s := make([]Result, 0)
	for _, site := range app.inventory {
		data, _ := app.results.Load(site.Domain)
		if data != nil {
			s = append(s, data.(Result))
		}
	}
	return s
}

func getJson(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response, _ := json.MarshalIndent(getResults(), "", " ")
	io.WriteString(w, string(response))
}

func getWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Println(err)
		}
		return
	}
	app.ws = ws

	reader(ws)
}

func reader(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadLimit(512)
	for {
		// msgType will be -1 for close, 1 for normal message
		msgType, msg, err := ws.ReadMessage()
		log.Print(msgType)
		log.Print(msg)
		if msgType == -1 {
			log.Print("chouf: info: received websocket close request from client")
			app.ws.Close()
			app.ws = nil
		}
		if err != nil {
			break
		}
	}
}

func getFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	http.ServeFile(w, r, "assets/favicon.ico")
}

func getJs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript")
	if os.Getenv("CHOUF_ENV") == "dev" {
		http.ServeFile(w, r, "assets/main.bundle.js")
		return
	}
	// we serve the brotli encoded file in prod
	w.Header().Set("Content-Encoding", "br")
	http.ServeFile(w, r, "assets/main.bundle.js.br")
}

func process(website Website) Result {
	// be optimistic by default
	ok := true
	// website specific endpoint
	endpoint := app.endpoint
	if len(website.Endpoint) > 0 {
		endpoint = website.Endpoint
	}

	// website specific status
	status := app.status
	if website.Status != 0 {
		status = website.Status
	}

	Debug(fmt.Sprintf("chouf: debug: expecting %d on %s", status, endpoint))
	// in 1.19, use net/url.JoinPath?
	url := fmt.Sprint("https://", path.Join(website.Domain, endpoint))
	client := &http.Client{}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		log.Printf("chouf: error: could not prepare request: %s", err)
	}
	// set User Agent
	req.Header.Set("User-Agent", fmt.Sprintf("chouf/%s", choufVersion))
	res, err := client.Do(req)
	if err != nil {
		log.Printf("chouf: error: could not execute request: %s", err)
	}
	if res.StatusCode != status {
		log.Printf("CHOUF: invalid status for %s: expected %d got %d", url, status, res.StatusCode)
		ok = false
	}
	return Result{
		Domain:    website.Domain,
		Ok:        ok,
		LastCheck: time.Now().Format(time.RFC3339),
	}
}

func readConfigFile(filePath string) (Yaml, error) {
	out := Yaml{}

	// read
	buf, err := ioutil.ReadFile(filePath)
	if err != nil {
		return out, err
	}

	// parse
	return out, yaml.Unmarshal(buf, &out)
}
