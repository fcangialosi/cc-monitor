all: ccperf cc-server db-server reader update-site

ccperf: client/*.go config/*.go results/*.go shared/*.go
	go build -o ./ccperf ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

db-server: db/*.go config/*.go results/*.go shared/*.go
	go build -o ./db-server ./db/

reader: db_client/*.go config/*.go results/*.go
	go build -o ./reader ./db_client/

release: client/*.go config/*.go results/*.go
	mkdir -p release/
	GOARCH=amd64 GOOS=linux go build -o release/ccperf-linux ./client
	#GOARCH=amd64 GOOS=darwin go build -o release/ccperf-darwin ./client
	GOARCH=amd64 GOOS=windows go build -o release/ccperf-windows ./client

UNAME := $(shell uname)
ifeq ($(UNAME), Darwin)
update-site:
	./update-binaries-from-mac.sh
else
endif


clean:
	rm -f ./ccperf
	rm -f ./cc-server
	rm -f ./db-server
	rm -f ./reader
	rm -rf release/
