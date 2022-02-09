# Sakura Gateway

## Installation
Install the dependencies
```
go get
```
Copy the .env.sample file as .env
```
cp .env.sample .env
```
Add your database information and other config to the .env file.

Note that the `ALLOWED_ORIGINS` field is used for site origins that are allowed to access the gateway (CORS).
```dotenv
ALLOWED_ORIGINS="http://hello.world, https://foo.bar"
```

## Usage
To run in a development environment:
```shell
go run cmd/gateway/main.go
```