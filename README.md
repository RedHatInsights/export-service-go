# export-service-go

## Starting the service
You can start the database and minio using `podman-compose up db s3`. This will expose minio on `localhost:9099` 
(use `minio` as the access key and `minioadmin` as the secret key)

Create a bucket called `exports-bucket` in minio, click **Manage** and set it's **Access Policy** to `public`.

Then start the export api service using the following command:
```
DEBUG=true \
MINIO_PORT=9099 \
AWS_ACCESS_KEY=minio \
AWS_SECRET_ACCESS_KEY=minioadmin \
PSKS=testing-a-psk \
PUBLICPORT=8000 \
METRICSPORT=9090 \
PRIVATEPORT=10010 \
PGSQL_PORT=5432 \
go run main.go
```

## Testing the service
You can create a new export request using `make sample-request-create-export` which pulls data from the `example_export_request.json`. It should respond with the following information:
```
{
    "id":"0b069353-6ace-4403-8162-3476df3ae4ab",
    "created":"2022-10-12T15:07:12.319191523Z",
    "name":"Example Export Request",
    "format":"json",
    "status":"pending",
    "sources":[{
        "id":"0b1386f4-2b91-44d7-bcb0-9391cfbba4c5","application":"exampleApplication",
        "status":"pending",
        "resource":"exampleResource",
        "filters":{}
    }]
}
```
Replace the `EXPORT_ID` and `EXPORT_RESOURCE` in the `Makefile` with the `id` and the sources `id` from the response.

You can then run `make sample-request-internal-upload` to upload `example_export_upload.zip` to the service. If this is successful, you should be able to download the uploaded file from the service using `make sample-request-export-download`.