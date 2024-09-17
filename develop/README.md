# How-Tos

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