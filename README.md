# mcnforwarder

[![asciicast](https://cloud.githubusercontent.com/assets/1476820/12314723/81091fa0-ba28-11e5-9b9b-c0b0f5824867.png)](https://asciinema.org/a/dgbbdvsx98g8ob2dab442p6o2)

Automatically forward exposed container ports for a Docker Machine to `localhost` (including the Docker API port).

Sometimes VBox host only interfaces are unreliable, the IPs are unreachable, or they are just plain hard to remember.

This program polls the Docker daemon via SSH and runs an SSH process that forwards any exposed container ports from the VM to your OSX or Windows machine.

Install: `go get github.com/nathanleclaire/mcnforwarder`.

Usage: `mcnforwarder [name]`.

It's new so there are still many rough edges, but is intended as a POC that might later be useful.
