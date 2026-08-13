# Linux complete server bundle

This bundle installs the OpenVPN Server and the `openvpn-web` management service on Debian/Ubuntu and RPM-family distributions. It is an online installer: the host package manager supplies OpenVPN, EasyRSA, iptables/nftables, jq and other runtime dependencies.

Every release publishes both kinds of Linux archive: `openvpn-web-linux-*` is the original Web-only binary package, while `openvpn-web-full-linux-*` is the complete native server bundle. The Web-only archive does not install OpenVPN Server.

```bash
tar -xzf openvpn-web-full-linux-x86_64.tar.gz
cd openvpn-web-full-linux-x86_64
sudo ./install.sh
```

Persistent data is stored in `/var/lib/openvpn-web`; upgrades keep the database, PKI, client profiles and `config.json`. Use `sudo ./uninstall.sh` to remove programs while preserving data, or `sudo ./uninstall.sh --purge --yes` to remove data too.

On the first native installation, the administrator account is `admin` and a random password is created in the root-only file `/etc/openvpn-web/initial-admin-password`:

```bash
sudo cat /etc/openvpn-web/initial-admin-password
```

Log in, change that password immediately in the Web UI, then remove the bootstrap file:

```bash
sudo rm -f /etc/openvpn-web/initial-admin-password
```

An upgrade with an existing `config.json` never creates or replaces the administrator password. The Docker deployment retains its existing initialization behavior; this password-file workflow applies only to the native Linux bundle and `.deb`/`.rpm` packages.

The installer uses systemd and requires root privileges, a TUN device, forwarding/firewall permissions, and an accessible UDP 1194 port. WSL can build and run static smoke checks; complete VPN validation should be done on a real Linux host or WSL systemd instance.

Do not run the native services and Docker deployment against the same ports or data directory at the same time.
