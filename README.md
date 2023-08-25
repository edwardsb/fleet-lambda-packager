# Fleet Lambda packager

1. build the go binary
2. build the docker image
3. TODO: lambda-ify

```shell
./build.sh
docker build -t fleet-packager:latest .
docker run fleet-packager:latest
## todo docker push ECR
```
