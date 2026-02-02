# Example files

[Example prometheus metric output](prometheus-data-example.txt)

[Example client configuration with localhost](config/local-test-config-client.yaml)

[Example server configuration with localhost](config/local-test-config-server.yaml)

[Example client configuration using legacy S2SProxyConfig](legacyconfig/old-config-with-override.yaml)

# How-Tos

## Setup a local proxy pair with two Temporal clusters

### Ports
| Service              | port  | Connected to   |
|----------------------|-------|----------------|
| Temporal-left        | 7233  | Temporal-left  |
| Outbound proxy-left  | 38233 | Temporal-right |
| proxy mux connection | 11000 | Other proxy    |
| Outbound proxy-right | 37233 | Temporal-left  |
| Temporal-right       | 8233  | Temporal-right |

### Steps
1. Start two local Temporal clusters, one on port 7233 and one on port 8233
   1. Make sure `enableGlobalNamespace: true` is in your config, and name them `left` and `right`
2. Start a proxy with `./bins/s2s-proxy start --config ./develop/config/local-test-config-server.yaml`
3. Start a proxy with `./bins/s2s-proxy start --config ./develop/config/local-test-config-client.yaml`
4. Add the proxy for Temporal-left
   1. `temporal --address localhost:7233 operator cluster upsert --frontend-address localhost:38233 --enable-connection`
5. Add the proxy for Temporal-right
   1. `temporal --address localhost:8233 operator cluster upsert --frontend-address localhost:37233 --enable-connection`
6. Create a namespace on Temporal-left and add Temporal-right as passive
   1. `temporal operator namespace create --active-cluster left --global -n left-ns`
   2. `temporal operator search-attribute create -n left-ns --name CustomStringField --type Text`
   3. `temporal operator namespace update -n left-ns --cluster left --cluster right`
7. Done! Create some workflows and run whatever tests you need. 

## Generate Lazy Client (hacky solution)

- Checkout https://github.com/temporalio/temporal
- Run
```
git checkout haifengh/auto-gen-lazy # which adds lazy client support in rpcwrappers
make service-clients
```
- Copy generated file over
```
cp ${temporal_path}/client/admin/lazy_client_gen.go client/admin/lazy_client_gen.go
cp ${temporal_path}/client/frontend/lazy_client_gen.go client/frontend/lazy_client_gen.go
```

## Build & Push docker image

```
make docker-build
make docker-login
make docker-push
```