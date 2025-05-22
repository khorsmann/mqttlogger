# mqttlogger

This Go application connects to an MQTT broker to receive energy<br/>
consumption data from sensors and stores the data in an SQLite database.<br/>
The application is designed to run indefinitely while listening for MQTT messages and handling clean-up upon termination.<br/>

# Features

 - Connects to an MQTT broker using provided configuration.
 - Subscribes to a specified MQTT topic.
 - Parses incoming JSON messages containing energy data.
 - Stores parsed data into an SQLite database. Generates it for you if not there.
 - Gracefully handles shutdown with resource cleanup upon receiving system termination signals.

# Configuration

The application requires a config.toml file for its configuration. Here's a sample configuration structure:

```bash
[broker]
host = "tcp://mqtt.example.com:1883"
username = "your_username"
password = "your_password"
client_id = "client_id_here"
topic = "energy/topic"
qos = 1

[database]
path = "path/to/database.db"
```

# systemd service

Copy the template to your config folder like this:

```bash
cp mqttlogger.service ~/.config/systemd/user/mqttlogger.service
```

Change the values to your needs and enable/start it.

```bash
systemctl --user daemon-reload
systemctl --user enable mqttlogger.service
systemctl --user start mqttlogger.service
# logging
journalctl --user -t mqttlogger-linux-arm64 -f
```

# Set german timezone on tasmota firmware

Go to Tools > Console and enter:
```bash
Timezone 99
```

