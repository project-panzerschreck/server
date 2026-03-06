# Linux Client

This is a simple client that announces itself to the server and runs an RPC server.

## Usage

The client takes the command name of llama.cpp's rpc server command and the IP of the tracker as arguments.
The rest of the arguments are passed to the rpc server command.
Use `--` to separate rpc-server's arguments from the client's arguments.

```bash
go run ./cmd/linux-client/ -cmd PATH/TO/rpc-server -tracker 127.0.0.1:4917 -- -c
```