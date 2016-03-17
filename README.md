# SignalR

This is a SignalR websocket connector written in Go.


## Getting Started
* This is developed using Go 1.5.1
* Pull the project with 'go get github.com/kcwinner/signalr'
* Compile with 'go install signalr.go'


## Parameters
* addr - endpoint to connect to
* hubname - SignalR hub to connect to
* logFile (optional) - the output for logs
* scheme (optional) - connection scheme: http or https


## Dependencies
golang.org/x/net/websocket
