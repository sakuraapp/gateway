.PHONY: cert

cert: ## Create certificates to encrypt the gRPC connection
	openssl genrsa -out cert/ca.key 4096
	openssl req -new -x509 -key cert/ca.key -sha256 -subj "/C=US/ST=NJ/O=Sakura, Inc." -days 365 -out cert/ca.cert
	openssl genrsa -out cert/service.key 4096
	openssl req -new -key cert/service.key -out cert/service.csr -config certificate.conf
	openssl x509 -req -in cert/service.csr -CA cert/ca.cert -CAkey cert/ca.key -CAcreateserial \
		-out cert/service.pem -days 365 -sha256 -extfile certificate.conf -extensions req_ext