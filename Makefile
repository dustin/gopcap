all : pcap.go decode.go io.go
	go tool fix $^
	go tool vet $^
	gofmt -s -w $^
	go build
	go install
test : all decode_test.go
	go test
