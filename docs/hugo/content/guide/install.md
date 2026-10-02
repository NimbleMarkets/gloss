---
title: "Install"
weight: 10
bookToC: false
---

# Install

With [Homebrew](https://brew.sh), on macOS or Linux:

```sh
brew install --cask nimblemarkets/tap/gloss
```

On Debian or Ubuntu, download the `.deb` for your machine from the
[latest release](https://github.com/NimbleMarkets/gloss/releases/latest) and install it:

```sh
sudo apt install ./gloss_*_linux_amd64.deb     # or _arm64
```

The release also has archives for macOS, Linux, and Windows (amd64 and arm64),
each a single binary with nothing else to install. With Go, `go install
github.com/NimbleMarkets/gloss/cmd/gloss@latest` builds it from source.

## For agents

If you are installing gloss for an AI agent, also install its skill, which
teaches the agent the headless interface (`--text`, `--output`, `--info`,
`--pick`, `--serve`):

```sh
gloss skill install
```

See [For agents](../agents/) for what the skill does and other ways to install it.
