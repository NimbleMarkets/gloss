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
You can also drop files onto the page. Review them in the viewer, then press
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
fetched from an address in a table can be picked too; it is then kept for you.

| Exit status | Meaning |
| --- | --- |
| 0 | Paths were printed |
| 1 | An error |
| 2 | Nothing was chosen |
| 124 | `--timeout` ran out |

## Without a terminal

When standard input is not a terminal, as when an agent starts gloss, `--pick`
and `--serve` do not block. gloss starts the server as a process of its own and
prints one JSON object on standard output, exiting 0, with no browser opened
(`--no-open` is then the default; on a terminal it is still opt-in):

```json
{"status":"waiting","url":"http://127.0.0.1:41233/<token>/","dir":"/tmp/gloss-pick-…","timeout_seconds":600,"resume_token":"<token>","resume":"gloss --resume <token>","pick":true}
```

`url` carries the token and is for the human; `dir` is the private folder
(mode 0700) where dropped files land; `--timeout` defaults to 10 minutes off a
terminal and bounds the server's life. `gloss --resume <token>` then waits for
the answer and gives it as a terminal pick does: the paths on standard output
(or `--json`, `{"status","paths","error"}`) and exit status 0, 2, or 124, from
any process, whether or not the one that started it is alive.
`--resume <token> --timeout 30s` bounds only the waiting (exit 124, with a
message saying the pick is still open), so a harness can poll; a settled pick
answers at once. An input that cannot be shown fails the start itself: exit 1,
nothing on standard output.

`gloss --status <token>` asks how the pick stands, at once, as one JSON object,
for a harness that wants to look between other work and not wait. It changes
nothing: it does not take the answer, end the session, or touch the files, so
it can be asked as often as wanted, and `--resume` then answers as it would have.

```json
{"state":"waiting","settled":false,"seconds_left":412}
{"state":"picked","settled":true,"paths":["/tmp/gloss-1234/invoice.pdf"]}
{"state":"failed","settled":true,"error":"the server ended without an answer"}
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
1 when there is no such pick: the token is not one, or the answer was collected
and its state removed.

Files handed over are kept for the caller, which must delete `dir` when done.
gloss deletes it on timeout, on a decline, and when the server dies unanswered,
but never after an answer.

Files dropped on a page are written to a folder of their own under the system's
temporary directory, readable by you alone. If they are the answer to a pick
they are left there for the program that asked, which must delete them when it
is done. Otherwise they are removed when gloss exits, and on a timeout.

The server listens on 127.0.0.1 only, on a port chosen at random. The page's
address carries a token, without which nothing is served, so other programs and
other pages cannot reach the viewer or drop files on it. `--no-open` prints the
address without opening a browser; `--timeout 10m` gives up after that long.
With `--serve` or `--pick`, standard input is read only when `-` is named.

Open the link on the machine running gloss, or through your environment's
supported local forwarding. A localhost link from a remote host or container
does not automatically work on another computer. Browsing with `o` and pasted
paths refer to files on the gloss machine; **Choose files** and browser drops
copy files from the computer running your browser.
