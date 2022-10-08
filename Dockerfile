# syntax=docker/dockerfile:1

# Dockerfile for chouf
# https://github.com/deltablot/chouf

# build the javascript/css bundle
FROM node:18-alpine AS jsbuilder
RUN apk add --no-cache brotli
USER node
WORKDIR /home/node
COPY --chown=node:node package.json .
COPY --chown=node:node package-lock.json .
COPY --chown=node:node webpack.config.js .
COPY --chown=node:node src src
RUN ls -la .
RUN npm ci && npm run build:prod

FROM golang:1.18-alpine AS gobuilder
# this is set at build time
ARG VERSION=docker
WORKDIR /app
# install dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download
# copy code
COPY src src
# disable CGO or it doesn't work in scratch
# target linux/amd64
# -w turn off DWARF debugging
# -s turn off symbol table
# change version at linking time
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -X 'main.choufVersion=${VERSION}'" -o /chouf ./src/go/*.go

# use distroless instead of scratch to have ssl certificates and nobody
FROM gcr.io/distroless/static
COPY --from=gobuilder /chouf /chouf
COPY --from=jsbuilder /home/node/assets/main.bundle.js* assets/
COPY assets/favicon.ico assets/favicon.ico
COPY src/tmpl/index.tmpl /src/tmpl/index.tmpl
USER nobody:nobody
EXPOSE 3003
ENTRYPOINT ["/chouf"]
CMD ["-c", "/etc/chouf/config.yml"]
