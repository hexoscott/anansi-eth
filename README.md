# anansi-eth
A trickster app to try and break Ethereum mempools

## Running
For now you will need to edit the code to adjust how many jobs run of each type using the `createJobs` function in `main.go`.

You can build the app with `go build` and then run it with `./anansi-eth`.

You will need to specify two flags:
- `-rpc-url`: The URL of the RPC to use
- `-private-key`: The private key of the account to use for funding
