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


