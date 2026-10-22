# gpstrack

ASI's vehicle tracking platform. This is **Phase 0**: a small gateway that
proves the 4G CJ720 tracker end to end before the real platform is built.

It:

- listens for trackers on **TCP and UDP 5013** and decodes **H02**, both the
  text form (`*HQ,...#`) and the binary form (`$...`) used by 4G units
- logs every byte in both directions to daily JSON Lines files
- decodes position reports (`V1`) and shows them on a status page with a map
- sends test commands, including the fuel cut, down the tracker's open
  connection, so we can find out what the 4G unit accepts over TCP rather than SMS

One Go binary with no dependencies. It runs on a Raspberry Pi.

## Quick start (laptop)

```sh
make test
GPSTRACK_TOKEN=devtoken go run ./cmd/gateway      # trackers :5013, page http://localhost:8080
go run ./cmd/sim -every 2s                        # fake CJ720 driving around Invercargill
```

Open http://localhost:8080 and enter the token. The simulator answers `S20`
commands with a `V4` reply and slows to a stop after a cut, like the fuel
lift pump's run-on.

## Raspberry Pi

**Full step-by-step guide: [docs/pi-setup.md](docs/pi-setup.md)**, from a blank SD card to a
tracker connecting over 4G.

Short version, on a Pi with Go installed:

```sh
git clone https://github.com/gomeznz/gpstrack.git && cd gpstrack
make build
sudo ./deploy/install-pi.sh bin/gpstrack-gateway
```

Or cross-compile on another machine with `make pi` (or `make pi32` for 32-bit
Pi OS) and copy `bin/arm64` and `deploy/` across.

The install script creates a `gpstrack` service user, a systemd service, data
in `/var/lib/gpstrack`, and `/etc/gpstrack/gpstrack.env` with a random token
(printed once). Logs: `journalctl -u gpstrack-gateway -f`.

### Network

- Port-forward **TCP and UDP 5013** on the router to the Pi. Nothing else.
  (The gateway listens on both; some H02 units may send over UDP.)
- Keep the status page (8080) on the LAN only.
- If using a Cloudflare hostname for the trackers, the DNS record must be
  **DNS only (grey cloud)**. Cloudflare's normal proxy carries web traffic,
  not a tracker's raw TCP.
- A fixed public IP is safest for the first test; see the hostname test below.

## Setting up a CJ730 Plus 4G (by SMS)

The 4G unit ASI is testing behaves like the **CJ730 Plus 4G (YunTrack)**: H02 on
port 5013, but it only answers SMS from its admin number, and the server is
set with a colon ([Traccar forum](https://www.traccar.org/forums/topic/cj730-plus-4g-yuntrack/)).

| Order | SMS | Notes |
| --- | --- | --- |
| 1 | `admin123456 +6421XXXXXXX` | Your mobile in **+64** format. The unit ignores other numbers after this |
| 2 | `apn123456 <One NZ IoT APN>` | CJ720 form; check it took in the `param1` reply |
| 3 | `adminip123456 <public IP>:5013` | **Colon**, not a space |
| 4 | `status` | e.g. `BAT:9,GPRS:1,GSM:5,GPS:0,ACC:0,oil:1,Power:1,S:1`. `GPRS:1` = data up, `oil:1` = fuel on |
| 5 | `param1` | Firmware, ID, server and APN |

- Won't accept commands? An admin number may already be set from another phone:
  `noadmin123456`, or `begin123456` to factory reset.
- Factory server, to send it back to the YunTrack app: `adminip123456 113.98.254.178:7700`.
- Fuel: `555` cuts, `666` restores (SMS). Over TCP, the forum reports the `S20` stop
  works but resume may not, so keep the `666` SMS handy when testing.
- 4G units often report positions as **binary H02** (starting `$`). The gateway decodes these.

## Setting up an original CJ720 (by SMS)

Default password is `123456` (fine for bench testing). Replace `<...>`:

| Order | SMS | Expected reply |
| --- | --- | --- |
| 1 | `admin123456 <your mobile>` | `admin ok` |
| 2 | `apn123456 <One NZ IoT APN>` | `apn ok` |
| 3 | `SZCS,123456,DOMAIN=<public IP> 5013` | `SET IP OK! ...` |
| 4 | `gprs123456` | `gprs ok` |
| 5 | `upload123456 10` | `upload ok` |
| 6 | `timezone123456 +0` | `time ok` |
| 7 | `param1` | firmware, ID, IP, port, APN |

Ask One NZ for the APN (and any username/password, set with `apnuser123456`
and `apnpasswd123456`) on the IoT plan. If `SZCS` isn't accepted, use
`adminip123456 <public IP> 5013` instead.

The unit should appear on the status page within a minute. Its ID is the
IMEI or the 10-digit ID on the label.

## Phase 0 checklist

Record the answers in the design spec.

- [ ] Unit connects and `V1` positions decode correctly (compare with `G123456#` by SMS)
- [ ] What else it sends: heartbeat, `LINK`, `NBR`, binary `$` frames? (all logged)
- [ ] Reporting interval when moving, when parked, and after `sleep123456 shock`
- [ ] Which status bits change for ignition (ACC), external power cut (`pwr1`), and relay cut — use the "0-bits" column with SMS `status` as the reference
- [ ] Which `S20` preset cuts the relay over TCP, and the exact reply
- [ ] Which preset restores it
- [ ] Relay state after the unit loses and regains power
- [ ] Whether `SZCS,123456,DOMAIN=<hostname> 5013` works, so servers can move without reprogramming units
- [ ] Data used per day on the One NZ SIM

## Data files

`raw/raw-YYYY-MM-DD.jsonl`: every frame in and out, plus connect and disconnect:

```json
{"time":"2026-10-21T23:11:30Z","dir":"out","remote":"203.0.113.5:40112","deviceId":"4106054111","text":"*HQ,4106054111,S20,231130,1,1#"}
```

Frames that aren't H02 text are logged as `hex`. `positions/` holds decoded
`V1` reports. Times are UTC.

## Layout

```
cmd/gateway     the gateway binary
cmd/sim         fake tracker for testing
internal/h02    H02 parser, stream framer, command encoder
internal/gateway TCP sessions, device state, command presets
internal/web    status page and JSON API
internal/store  daily JSON Lines files
deploy/         systemd unit, env example, Pi install script
```

## API

Bearer token required when `GPSTRACK_TOKEN` is set; without a token the page
is read-only and commands are disabled.

- `GET /api/devices`: latest state per device
- `GET /api/log?n=200`: newest raw log entries
- `POST /api/devices/{id}/command`: `{"preset":"cut"}` or `{"raw":"*HQ,...#"}`
