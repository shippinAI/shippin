BINARY = warden
INSTALL_DIR = /usr/local/bin
SERVICE_FILE = warden.service
SYSTEMD_DIR = /etc/systemd/system
CONFIG_DIR = /etc/warden

.PHONY: build install uninstall enable start stop status logs

build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY) .

install: build
	@echo "Installing $(BINARY)..."
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	install -m 644 $(SERVICE_FILE) $(SYSTEMD_DIR)/$(SERVICE_FILE)
	mkdir -p $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/warden.env ]; then \
		echo "# Warden configuration" > $(CONFIG_DIR)/warden.env; \
		echo "# PORT=9006" >> $(CONFIG_DIR)/warden.env; \
		echo "# TLS=true" >> $(CONFIG_DIR)/warden.env; \
		echo "# TLS_CERT=/etc/Warden/tls/cert.pem" >> $(CONFIG_DIR)/warden.env; \
		echo "# TLS_KEY=/etc/Warden/tls/key.pem" >> $(CONFIG_DIR)/warden.env; \
		chmod 600 $(CONFIG_DIR)/warden.env; \
	fi
	systemctl daemon-reload
	@echo ""
	@echo "Warden installed. To start:"
	@echo "  sudo systemctl enable warden"
	@echo "  sudo systemctl start warden"

uninstall:
	@echo "Removing $(BINARY)..."
	-systemctl stop warden 2>/dev/null
	-systemctl disable warden 2>/dev/null
	rm -f $(SYSTEMD_DIR)/$(SERVICE_FILE)
	rm -f $(INSTALL_DIR)/$(BINARY)
	systemctl daemon-reload
	@echo ""
	@echo "Warden removed. Config preserved at $(CONFIG_DIR)/"

enable:
	systemctl enable warden

start:
	systemctl start warden

stop:
	systemctl stop warden

status:
	systemctl status warden

logs:
	journalctl -u warden -f
