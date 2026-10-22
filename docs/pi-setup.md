# Raspberry Pi setup

From a blank SD card to a gateway that a CJ720 can reach over 4G. Takes
about 30 minutes.

**You need:** a Raspberry Pi 4 or 5, a 16 GB+ microSD card, a network cable
(Wi-Fi works but wired is more reliable), and a computer to set it up from.

## 1. Put Raspberry Pi OS on the card

1. Install **Raspberry Pi Imager** from <https://www.raspberrypi.com/software/> and open it.
2. **Device:** your Pi model.
   **OS:** *Raspberry Pi OS (other)* → **Raspberry Pi OS Lite (64-bit)**. No desktop is needed.
   **Storage:** the SD card.
3. Click **Next**, then **Edit settings** when asked about customisation:
   - **General:** hostname `gpstrack`, a username and password you will remember,
     time zone `Pacific/Auckland`.
   - **Services:** tick **Enable SSH** → *Use password authentication*.
4. Save, write the card, put it in the Pi, plug in the network cable and power.

## 2. Log in

Wait two minutes for the first boot, then from your computer:

```sh
ssh <username>@gpstrack.local
```

If `gpstrack.local` isn't found, look up the Pi's IP address in your router's
client list and use `ssh <username>@<ip>` instead.

## 3. Give the Pi a fixed LAN address

In your router, find the Pi in the DHCP client list and add a **DHCP
reservation** (sometimes called "static lease") so it always gets the same IP,
e.g. `192.168.1.50`. You need a fixed address for the port forward in step 7.

## 4. Update and install tools

Do this over **SSH**, not the Raspberry Pi Connect remote shell. Updating
`rpi-connect` restarts the service the Connect shell runs through, which
cuts the upgrade off half-way and leaves `dpkg was interrupted` errors. If
that happens, SSH in and run `sudo dpkg --configure -a`.

On the Pi:

```sh
sudo apt update && sudo apt full-upgrade -y
sudo apt install -y git make
```

Install Go. The Pi OS package is too old, so use the official build:

```sh
wget https://go.dev/dl/go1.24.7.linux-arm64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.7.linux-arm64.tar.gz
rm go1.24.7.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile
go version        # should print go1.24.7 linux/arm64
```

## 5. Get the code

The repo is private, so GitHub needs a login. The simplest way is a
**personal access token**: on GitHub go to *Settings → Developer settings →
Personal access tokens → Fine-grained tokens*, create one with **read access to
Contents** on `gomeznz/gpstrack`, and copy it.

```sh
git clone https://github.com/gomeznz/gpstrack.git
cd gpstrack
```

When asked, the username is `gomeznz` and the password is the token. To stop
it asking every time: `git config --global credential.helper store` before
cloning (the token is then saved in `~/.git-credentials`).

## 6. Build and install

Still in `~/gpstrack`:

```sh
make test         # optional: runs the test suite
make build
sudo ./deploy/install-pi.sh bin/gpstrack-gateway
```

The install script:

- creates a `gpstrack` system user
- copies the gateway to `/opt/gpstrack/`
- creates `/etc/gpstrack/gpstrack.env` with a random **token**, printed on
  screen. Note it down; it's the login for the status page
- stores data in `/var/lib/gpstrack/`
- starts the `gpstrack-gateway` service and sets it to start on boot

Check it's running:

```sh
systemctl status gpstrack-gateway        # should say "active (running)"
journalctl -u gpstrack-gateway -f        # live log; Ctrl+C to stop watching
```

Forgot the token? `sudo grep TOKEN /etc/gpstrack/gpstrack.env`

## 7. Test it on the LAN

On your computer, open `http://gpstrack.local:8080` (or `http://<pi-ip>:8080`),
enter the token and click **Unlock**.

Then, on the Pi, start the fake tracker:

```sh
./bin/gpstrack-sim -addr 127.0.0.1:5013 -every 2s
```

Device `4106054111` should appear on the page within a few seconds and start
moving around Invercargill. Try **Cut fuel (S20 1,1)**: the log shows the
command going out, the `V4` reply coming back, and the speed dropping to 0.
Press **Ctrl+C** to stop the simulator.

## 8. Open the tracker port to the internet

The tracker connects in over 4G, so your router must pass port **5013** to the Pi:

1. In the router, add a **port forward**: external `5013` → `<pi-ip>` port
   `5013`, protocol **TCP and UDP** (some routers call it "Both"). Do **not**
   forward 8080.
2. Find your public IP: `curl -4 ifconfig.me` on the Pi.
3. Test from outside your network, e.g. a laptop tethered to a phone on mobile data:
   ```sh
   nc -vz <public-ip> 5013        # "succeeded" / "open" = good
   ```
   A "connect" line also appears in the Pi's raw traffic log.

If your connection's public IP changes (most home and some business plans),
ask your ISP for a static IP, or use a hostname. With Cloudflare, the DNS
record for the tracker must be **DNS only (grey cloud)**, not proxied.

## 9. Point the CJ720 at the Pi

Follow the SMS setup in the [README](../README.md) for your model (CJ730 Plus
4G or original CJ720), using the public IP from step 8. Once `status` shows
`GPRS:1`, the unit should show up on the status page within a minute or two.

Browsers and port scanners on the internet will also hit port 5013. The log
labels those ("HTTPS handshake ... not a tracker"); the tracker is the one
whose entries carry a device ID.

## Updating

```sh
cd ~/gpstrack
git pull
make build
sudo ./deploy/install-pi.sh bin/gpstrack-gateway
```

The token, settings and logged data are kept.

## Changing settings

Edit `/etc/gpstrack/gpstrack.env` (ports, token), then
`sudo systemctl restart gpstrack-gateway`.

## Getting the logs off the Pi

From your computer:

```sh
scp -r <username>@gpstrack.local:/var/lib/gpstrack/raw ./captures/
```

If permission is refused, on the Pi first run
`sudo cp -r /var/lib/gpstrack/raw ~/captures && sudo chown -R $USER ~/captures`
and copy `~/captures` instead.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Page won't load | `systemctl status gpstrack-gateway`; right IP; port `:8080` in the address |
| Page asks for the token again | Token typed wrong. Get it with `sudo grep TOKEN /etc/gpstrack/gpstrack.env` |
| Simulator works, tracker never appears | Port forward (step 8), `nc` test from outside, then SMS `param1` to check the IP, port and APN on the unit |
| Tracker connects then drops | Look for "not a text frame" lines in the log: the unit may be sending binary; save the log for analysis |
| `go: command not found` | Run `source ~/.profile` or log out and back in |
| Service fails to start | `journalctl -u gpstrack-gateway -n 50` for the error. Another program on port 5013 or 8080 is the usual cause |

## Removing it

```sh
sudo systemctl disable --now gpstrack-gateway
sudo rm -rf /opt/gpstrack /etc/gpstrack /etc/systemd/system/gpstrack-gateway.service
sudo systemctl daemon-reload
# Data is left in /var/lib/gpstrack; delete it too if you don't need it.
```
