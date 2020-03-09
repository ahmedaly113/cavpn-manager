# cavpn-manager

Daemon for configuring peers for CaVPN interfaces, and collect metrics from CaVPN

This project is not affiliated with the CaVPN project.
CaVPN is a registered trademark of Jason A. Donenfeld.

## Building

Clone this repository, and run `make` to build.
This will produce a `cavpn-manager` binary and put them in your `GOBIN`.

## Testing
To run the tests, run `make test`.
To run the integration tests as well, run `go test ./...`. Note that this requires CaVPN to be running on the machine, and root privileges.

### Testing iptables using network namespaces
To test iptables without messing with your system configuration, you can use network namespaces.
To set one up and enter it, run the following commands:

```
sudo ip netns add cv-test
sudo -E env "PATH=$PATH" nsenter --net=/var/run/netns/cv-test
```

Then you can run the tests as described above.

## Usage
All options can be either configured via command line flags, or via their respective environment variable, as denoted by `[ENVIRONMENT_VARIABLE]`.
To get a list of all the options, run `cavpn-manager -h`.

When installed via the `.deb` package, a user named `cavpn-manager` will be created for the service to run as, as well as a systemd service named `cavpn-manager.service`.
The name of the binary when installed via the `.deb` package is `cavpn-manager`.
Configuration is done by creating a file at `/etc/default/cavpn-manager` and defining the environment variables there.
All logs are sent to stdout/stderr, so in order to debug issues with the service, simply use `journalctl` or `systemctl status`.

## Packaging
In order to deploy cavpn-manager, we build `.deb` packages. We use docker to make this process easier, so make sure you have that installed and running.
To create a new package, first create a new tag in git, this will be used for the package version:
```
git tag -s -a v1.0.0 -m "1.0.0"
```
Then, run `make package`. This will output the new package in the `build` folder.
Don't forget to push the tag to git afterwards.
