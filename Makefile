all: cc-client cc-server db-server reader

cc-client: client/*.go config/*.go results/*.go
	go build -o ./cc-client ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

db-server: db/*.go config/*.go results/*.go
	go build -o ./db-server ./db/

reader: db_client/*.go config/*.go results/*.go
	go build -o ./reader ./db_client/
clean:
	rm -f ./cc-client
	rm -f ./cc-server
	rm -f ./db-server
	rm -f ./reader
