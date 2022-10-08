# Chouf

## Description

Chouf takes a list of websites and check for their response status code periodically. It runs in the background and will emit log messages when a server doesn't respond correctly.

An HTTP server is started and will replay with a JSON dump of results.

## Usage

### Running chouf

You need two files to run chouf:

* `config.yml`: chouf configuration file containing the sites to check
* `docker-compose.yml`: your docker-compose file

#### Step 0: Chouf config
Create a `config.yml` file to hold chouf configuration.

See [config.yml example](./config.dist.yml).

#### Step 1: Docker-compose config

See [docker-compose.yml example](./docker-compose.dist.yml).

#### Step 2: Launch service

Once your config files are in place:

~~~bash
docker-compose up -d
~~~

### Using chouf

~~~
# get output as json
curl http://localhost:3003/json
~~~
