# export-service-go

## Dependencies
- Golang >= 1.16
- podman-compose
- make (optional for running the Makefile)
## Starting the service
You can start the database and minio using `podman-compose up db s3`. This will expose minio on `localhost:9099` 
(use `minio` as the access key and `minioadmin` as the secret key)

Create a bucket called `exports-bucket` in minio, click **Manage** and set it's **Access Policy** to `public`.

Then start the export api service locally using `make run`

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

## Integrating with the export service
The basic process of using the export service is as follows:

![Requesting an export](docs/request-export.png)

In order for your service act as an **export source** for the **export service** as shown above, your service must consume events from the `platform.export.requests` topic. The **export service** will send a message to this topic when a new export request is created. This message will contain the following information:

- `uuid`: identifier for the export request
- `application`: identifier for the application a request is being made for
- `resource`: identifier for the resource a request is being made for
- `format`: the format the export should be in. `json` or `csv` (`pdf` is not supported yet)
- `x-rh-identity`: the auth header of the user that requested the export. Can be used for logging, or for custom authorization/filtering by the app.
- `filters`: application-specific, schemaless `json` object used for filtering the data to be exported. This is not required. (not supported yet)
