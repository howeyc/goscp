set GOPATH=%BuildFolder%
set GOARCH=386
go get -v
go build -v -o goscp_386.exe
set GOARCH=amd64
go build -a -v -o goscp_amd64.exe
