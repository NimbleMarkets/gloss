---
title: "Asking for a file"
weight: 30
bookToC: false
---

# Asking for a file

A program that cannot show a terminal, such as an agent working for you, can
still ask you for a file, or show you one:

```sh
gloss --serve --pick             # prints the paths you send, one to a line
gloss --serve report.pdf         # shows you the file; prints nothing
gloss --pick                     # the same question, asked in the terminal
```

`--serve` starts a temporary server on this machine, opens the viewer on a page
in your browser, and ends when you quit the viewer or close the tab. The page is
the native viewer, not the demo: `o` browses your own folders, and a pasted path
is read from your disk. Files dropped on the page are handed to gloss.

`--prompt "Choose the March invoice so I can check its totals"` tells you what
the caller needs and why. In the browser it is a persistent heading above the
viewer, with a **Choose files** button that opens your browser's file picker.
You can also drop files onto the page. A choice or drop of more than 200 files
reads only the first 200, and says how many it left out. Review them in the viewer, then press
`Enter` there to send them; choosing or dropping files alone does not send
the answer. Press `q` in the viewer to decline.

In a terminal, the prompt is a bold box with an accent border below the viewer;
`--prompt-loc top` places it above instead. The terminal box wraps to at most
four lines and is hidden when the screen is too small. The browser heading
keeps the full request independently of the terminal's size.

`--pick` waits for you to hand files over by dropping them, pasting their
paths, or choosing them with `o`. The viewer shows what you gave and says what
`Enter` will send; `Enter` sends it and quits, and `q` sends nothing. With files
named on the command line and none handed over, `Enter` sends the one on screen.
Standard output carries only the answer, as full paths. In a terminal the viewer
draws on the terminal itself, so the answer can be piped. With `--fetch`, a file
fetched from an address in a table or a dropped URL can be picked too; it is then kept for you.

| Exit status | Meaning |
| --- | --- |
| 0 | Paths were printed |
| 1 | An error |
| 2 | Nothing was chosen |
| 124 | `--timeout` ran out |

## Plain web picker (experimental)

```sh
gloss --pick-web --prompt "Choose the March invoice" --accept pdf --timeout 10m
```

`--pick-web` is an upload-only alternative to the terminal viewer. It implies
`--serve --pick` and shows an ordinary HTML/JavaScript page with **Choose files**,
upload progress, a file list, **Remove**, **Send files**, and **Cancel**. It needs
no terminal connection or WASM. Small browser-supported images get local
thumbnails; other files show their names and sizes. It does not browse the
server's folders or provide downloads of its files.

Files upload to the computer running gloss as you choose or drop them. They
become the answer only when you press **Send files**. **Remove** deletes an
upload, and **Cancel** declines the request and removes its uploads. Refreshing
the page recovers completed uploads (names and sizes; local thumbnails are not
retained). Closing the tab leaves the request waiting; use `--timeout` to bound
its lifetime. A confirmation ends the server, so the finished link is not a
lasting receipt; the requester can check `--status` or `--resume`.

**Message to requester** is an optional reply sent with the selected files.
It accepts up to 2,000 Unicode characters, including line breaks. At least one
file is still required. The draft stays in this browser tab and survives a
refresh when browser session storage is available; it is sent only with
**Send files**, never with **Cancel**. A successful send or cancel clears it.

Use `gloss --resume TOKEN --json` to collect both paths and the message:

```json
{"protocol":1,"status":"picked","paths":["/tmp/gloss-pick-…/receipt.pdf"],"message":"Page 2 is missing.","error":""}
```

`--status TOKEN` also includes `message` once picked. Empty or whitespace-only
messages are omitted. Repeated status/resume calls retain the same message;
plain-text output remains file paths only. With terminal stdin,
`--pick-web --json` prints the same object on confirmation. This is a message
from the person sending files, distinct from the requester's `--prompt`.

The spike accepts at most 200 files, 128 MiB per file, and 1 GiB total. Uploads
have a two-minute deadline. `--accept` checks content formats; this page does
not run document renderers to validate or preview uploads. It takes no initial
files, `--glob`, `--fetch`, `--menu`, or `--preview`. The existing viewer remains
available with `--serve --pick`.

With terminal stdin it opens a browser and waits, printing the confirmed full
paths; `--no-open` prints the address on stderr instead of opening it. Without
terminal stdin it detaches, with the same startup JSON, `--status`, `--resume`,
`--cancel`, exit codes, and cleanup described below. It listens on localhost
unless you explicitly select a network address as described next.

### LAN and Tailscale addresses

Only the upload-only `--pick-web` page supports network binding. The full
`--serve` viewer, including its host-file browser, stays localhost-only.

```sh
# Replace this example with an IP assigned to the computer running gloss.
gloss --pick-web --listen 192.168.1.42:0 --no-open --timeout 10m \
  --prompt "Send a photo and tell me what to look at"

# Listen on all IPv4 addresses, sharing one explicit address with the human.
gloss --pick-web --listen 0.0.0.0:0 --advertise-host 192.168.1.42 --no-open

# Use a MagicDNS name in the link (the phone must have tailnet access).
gloss --pick-web --listen 0.0.0.0:0 \
  --advertise-host laptop.example-tailnet.ts.net --no-open
```

`--listen` takes a **literal IP and port**, defaulting to `127.0.0.1:0`.
Port `0` chooses an available port; specify a fixed one if your firewall or
tailnet policy requires it. IPv6 uses brackets: `--listen '[fd00::42]:0'`.
`0.0.0.0` listens on all IPv4 addresses; `[::]` listens on all IPv6 addresses.
Neither is a destination you can put in a browser. Interface names such as
`en0`, IPv6 zone identifiers, and link-local addresses are not supported.
Use an assigned unicast address, or an explicit wildcard.

`--advertise-host` takes an IP or ASCII DNS name, without a scheme, port, or
path. It defaults to a specific listen IP, and is **required for a wildcard**.
Gloss puts this host, the actual listening port, and the session token in the
printed URL and detached startup JSON. An advertised IP must match a specific
listen IP, or have the same address family as a wildcard. Names are not
resolved by gloss: the browser must resolve them to an address the listener
accepts. Specifying a name does not configure DNS, a firewall, or a tunnel.

With a specific listener, requests may use its IP or the advertised name;
loopback listeners also accept `localhost`. With a wildcard, only the
advertised host is accepted. The actual port must match. An advertised
Tailscale name does not make a wildcard listener Tailscale-only: it still
listens on LAN addresses, and an HTTP Host header is not a network firewall.
Where supported, binding to the computer's Tailscale IP narrows the listening
address. The phone needs Tailscale connectivity, DNS, and a tailnet policy
that permits the connection. Prefer the full MagicDNS name for shared links.
See [MagicDNS](https://tailscale.com/docs/features/magicdns).

The page uses HTTP. The token controls access but does not encrypt LAN
uploads. Keep the link private and use a network you trust. HTTPS reverse
proxies (including Tailscale Serve) are not supported by these flags; gloss
does not trust forwarded host or protocol headers. Use `--no-open` when the
link is intended for another device. The printed URL is a candidate, not a
reachability test: check it from that device; Wi-Fi client isolation and
firewalls can still block it.

When stdin and stderr are terminals, a network request automatically shows
the URL as a QR code while it waits. This works with `--no-open`: scan the code
from a device that can reach the advertised address, choose files, add an
optional message, then press **Send files** on that device. `g` switches Kitty
graphics and the colored half-block fallback; `q`, `Esc`, or Ctrl-C cancels the
request. If the whole code cannot fit, enlarge the terminal; gloss never crops
it or removes its quiet zone. `--render glyph` forces the fallback.

The QR screen closes and restores the terminal after confirmation,
cancellation, or timeout. It draws on stderr, so stdout contains only the
answer (paths, or JSON with `--json`). The full URL is printed before the screen
opens and remains in terminal history. Loopback requests do not show a phone
QR. Detached starts still print only their startup JSON; redirected stderr
stays plain text. No terminal UI or escape sequences are added to agent output.

## Required formats

Use `--accept 'image/*'` for images (including SVG), or a comma-separated list
of gloss format names such as `--accept 'image/*,pdf'`. It applies to initial
files, file choices, drops, and downloads. Content is checked independently of
`--type`; renaming HTML to `.png` cannot satisfy an image requirement. A mismatch
reports what arrived and what was required; it is rejected and cannot be picked.
Rejected downloads are deleted. Text formats without a distinctive signature
use their filename hint; document loaders still validate the format itself.

`--fetch` enables dropping one http(s) URL at a time. Browser URL drops fetch
directly from the remote server with CORS, without cookies or a referrer, with
a one-minute deadline and the same file size limit. They never use a site proxy
or bypass CORS. In the public app and landing-page demo, URL drops are enabled;
`app.html?accept=image%2F*` restricts an app session to images. Fetched documents
are not saved in the public app's persistent library.

## Without a terminal

When standard input is not a terminal, as when an agent starts gloss, `--pick`
and `--serve` do not block. gloss starts the server as a process of its own and
prints one JSON object on standard output, exiting 0, with no browser opened
(`--no-open` is then the default; on a terminal it is still opt-in):

```json
{"protocol":1,"status":"waiting","url":"http://127.0.0.1:41233/<page-token>/","dir":"/tmp/gloss-pick-…","timeout_seconds":600,"resume_token":"<token>","resume":"gloss --resume <token>","pick":true}
```

`url` carries a token of its own and is for the human; it lets a browser in and
nothing more. `resume_token` is a different secret, and the only one that
`--resume`, `--status`, and `--cancel` take. `protocol` is the version of these
objects' shape; `gloss skill schema` prints their JSON Schema. `dir` is the private folder
(mode 0700) where dropped files land; `--timeout` defaults to 10 minutes off a
terminal and bounds the server's life. `gloss --resume <token>` then waits for
the answer and gives it as a terminal pick does: the paths on standard output
(or `--json`, `{"protocol","status","paths","error"}`) and exit status 0, 2, or 124, from
any process, whether or not the one that started it is alive.
`--resume <token> --timeout 30s` bounds only the waiting (exit 124, with a
message saying the pick is still open), so a harness can poll; a settled pick
answers at once. An input that cannot be shown fails the start itself: exit 1,
nothing on standard output.

`gloss --status <token>` asks how the pick stands, at once, as one JSON object,
for a harness that wants to look between other work and not wait. It changes
nothing: it does not take the answer, end the session, or touch the files, so
it can be asked as often as wanted, and `--resume` then answers as it would have.
Collecting the answer does not end the session: `--status` still reports the
settled state, paths and all, and `--resume` gives the answer again.

```json
{"protocol":1,"state":"waiting","settled":false,"seconds_left":412}
{"protocol":1,"state":"picked","settled":true,"paths":["/tmp/gloss-1234/invoice.pdf"]}
{"protocol":1,"state":"failed","settled":true,"error":"the server ended without an answer"}
```

| `state` | Meaning |
| --- | --- |
| `waiting` | The viewer is open; `seconds_left` is about how long the pick has |
| `picked` | Files were sent; `paths` lists them |
| `declined` | The person declined (`q`, or Cancel) |
| `timeout` | `--timeout` ran out, or the deadline passed with the server gone |
| `closed` | A page that only showed something was closed |
| `failed` | An error, said in `error`; a server that died unanswered is one |

A state that is out of date on disk (its deadline has passed, or its server has
died) is reported as what it has become, without being rewritten. The exit
status is 0 whenever a state was reported, because the state is in the JSON, and
1 when there is no such session: the token is not one, or the session was
cancelled.

`gloss --cancel <token>` ends a session at once: the server, if it is still
waiting, stops; `dir` is deleted with every file dropped there, picked or not;
and the session is forgotten, so `--status` and `--resume` then exit 1. It
prints nothing and exits 0, or 1 for no such session.

Files handed over are kept for the caller, which deletes `dir` when done, or
runs `--cancel`. gloss deletes it on timeout, on a decline, on `--cancel`, and
when the server dies unanswered, but never after an answer otherwise. A
session nobody cancels keeps its state for a day after its timeout, for
`--status` and `--resume`; a later start then removes it.

Files dropped on a page are written to a folder of their own under the system's
temporary directory, readable by you alone. If they are the answer to a pick
they are left there for the program that asked, which must delete them when it
is done. Otherwise they are removed when gloss exits, and on a timeout.

The full viewer listens on 127.0.0.1 only, on a port chosen at random.
`--pick-web` uses the same default but supports the explicit LAN settings above.
The page's address carries a token, without which nothing is served.
`--no-open` prints the
address without opening a browser; `--timeout 10m` gives up after that long.
With `--serve` or `--pick`, standard input is read only when `-` is named.

Open the link on the machine running gloss, or through your environment's
supported local forwarding. A localhost link from a remote host or container
does not automatically work on another computer. Browsing with `o` and pasted
paths refer to files on the gloss machine; **Choose files** and browser drops
copy files from the computer running your browser.
