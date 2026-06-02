---
title: Go
summary: >-
  `go install` compiles mdsmith from the tagged
  module source with the host Go 1.24+ toolchain; no
  prebuilt binary is downloaded.
mechanism: toolchain
artifact: cli
command: go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
audience: Go developers with a working Go toolchain
platforms: [go]
channelurl: https://pkg.go.dev/github.com/jeduden/mdsmith
weight: 1
---
# Go

Release page: <https://pkg.go.dev/github.com/jeduden/mdsmith>

`go install` builds mdsmith from the tagged module
source. The host Go toolchain does the compile, so no
prebuilt binary is fetched. The version is stamped
from the module tag. `mdsmith version` then matches
every other channel. This path needs Go 1.24 or
newer.
