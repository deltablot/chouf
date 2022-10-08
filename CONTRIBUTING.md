# Contributing

## Setting up dev env

Fork it, clone it.

~~~bash
cd chouf
go mod download
~~~

## Launching chouf

~~~bash
# start it in debug mode
go build && ./chouf -c config.yml -d
# see available options with
chouf -h
~~~

## Building docker image

~~~bash
docker buildx build -t deltablot/chouf --build-arg VERSION=X.Y.Z .
~~~
