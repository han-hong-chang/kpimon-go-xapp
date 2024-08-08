
**Edit Configuration:**

Note: you need to ckeck for the platform configuration, especially the influxDB section.
```
vim kpimon-go/deploy/config.json
```
```
...
"influxDB":{
    "influxDBAddress": "http://r4-influxdb-influxdb2.ricplt:80",
    "username": "admin",
    "password": "7jQCNdujbSKju7cL32IzOOwAx7rEjEGJ",
    "token": "Y7zuwNFbRMyHWC6oecIntPEUN04aar78",
    "organization": "my-org",
    "bucket": "kpimon"
  }
...
```

**Build docker image:**

```
cd kpimon-go
sudo docker build -t nexus3.o-ran-sc.org:10004/o-ran-sc/ric-app-kpimon-go:1.0.1 .
```

**Onboard xApp Configuration:**

```
cd kpimon-go/deploy
dms_cli onboard config.json schema.json
```

**Deploy xApp:**

```
dms_cli install kpimon-go 2.0.1 ricxapp
```

**Undeploy xApp:**

```
dms_cli uninstall kpimon-go ricxapp 2.0.1
```# kpimon-go-xapp
