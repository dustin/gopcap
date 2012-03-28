all : pcap.go decode.go io.go
	go tool fix $^
	go tool vet $^
	gofmt -s -w $^
	go build
	go install
