# anansi-eth

A trickster app to try and break Ethereum mempools

![image](./anansi-bot.webp)

## Running

For now you will need to edit the code to adjust how many jobs run of each type using the `createJobs` function in `main.go`.

You can build the app with `go build` and then run it with `./anansi-eth`.

You will need to specify two flags:

- `-rpc-url`: The URL of the RPC to use
- `-private-key`: The private key of the account to use for funding
- `-job-file`: The file determining the jobs to load, defaults to ./config/all.json which loads 2 of each job type.

## Nix

Automatic installation of all required packages for development if `direnv` is installed. In case not, please install nix package manager(https://nixos.org/download/) and run `nix develop` in the project directory.
