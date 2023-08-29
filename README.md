# Fleet Lambda packager

1. build the go binary
2. build the docker image
3. TODO: lambda-ify

```shell
./build.sh
docker build --platform linux/amd64 -t fleet-packager:latest .
docker run fleet-packager:latest
## todo docker push ECR
```

Install local lambda container runtime:
```shell
mkdir -p ~/.aws-lambda-rie && \
    curl -Lo ~/.aws-lambda-rie/aws-lambda-rie https://github.com/aws/aws-lambda-runtime-interface-emulator/releases/latest/download/aws-lambda-rie && \
    chmod +x ~/.aws-lambda-rie/aws-lambda-rie
```

Build packager docker image:
```shell
docker build --platform linux/amd64 -t fleet-packager:latest .
```

Launch the lambda container runtime & tail the logs:
```shell
docker logs -f $(docker run -d -v ~/.aws-lambda-rie:/aws-lambda -p 9000:8080 \
    --entrypoint /aws-lambda/aws-lambda-rie \
    fleet-packager:latest \
        ./packager)
```

Invoke the lambda:
```shell
curl "http://localhost:9000/2015-03-31/functions/function/invocations" -d '{}'
```