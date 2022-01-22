module github.com/sakuraapp/gateway

go 1.16

replace github.com/sakuraapp/shared => /Users/jackie/Documents/Projects/Sakura/Source/shared

require (
	github.com/go-pg/pg/extra/pgdebug v0.2.0
	github.com/go-pg/pg/v10 v10.10.6
	github.com/go-redis/cache/v8 v8.4.3
	github.com/go-redis/redis/v8 v8.11.5-0.20211027084822-25378ca292e5
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/google/uuid v1.3.0
	github.com/joho/godotenv v1.3.0
	github.com/lesismal/nbio v1.2.2
	github.com/mitchellh/mapstructure v1.4.3
	github.com/rs/cors v1.8.0
	github.com/sakuraapp/shared v0.0.0-20220122195140-0a094c93b366
	github.com/sirupsen/logrus v1.8.1
	github.com/vmihailenco/msgpack/v5 v5.3.4
	golang.org/x/net v0.0.0-20211020060615-d418f374d309
)
