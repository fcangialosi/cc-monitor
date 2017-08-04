all: cc-client cc-server db-server reader

cc-client: client/*.go config/*.go results/*.go
	go build -o ./cc-client ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

db-server: db/*.go config/*.go results/*.go
	go build -o ./db-server ./db/

reader: db_client/*.go config/*.go results/*.go
	go build -o ./reader ./db_client/

release: client/*.go config/*.go results/*.go
	mkdir -p release/
	GOARCH=amd64 GOOS=linux go build -o release/cc-client-linux ./client
	GOARCH=amd64 GOOS=darwin go build -o release/cc-client-darwin ./client
	GOARCH=amd64 GOOS=windows go build -o release/cc-client-windows ./client

clean:
	rm -f ./cc-client
	rm -f ./cc-server
	rm -f ./db-server
	rm -f ./reader
	rm -rf release/
