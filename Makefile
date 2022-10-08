all: web build

build:
	go build -o bin ./src/go/*.go

dev:
	make build && bin/chouf -c config.yml -T 30s -d

docker:
	docker buildx build -t deltablot/chouf --build-arg VERSION=1.0.0 .
	dc up -d

run:
	./bin/chouf -c config.yml

serve: web build
	bin/chouf -c config.yml

web:
	npm run build:prod
