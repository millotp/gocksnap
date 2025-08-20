# gocksnap [![GitHub release](https://img.shields.io/badge/version-v0.1-green.svg?style=flat)](https://github.com/millotp/gocksnap/releases)

Jest-like snapshot built on top of [gock](https://github.com/h2non/gock).

It allows you to record HTTP request for a test, select the response for each one and replay the snapshot in the test.

Instead of having to manually register all the mocks, they will be discoverd automatically, so you don't need all the `gock.New` or `gock.Register` calls anymore !

## Example

```go
func TestWithHTTP(t *testing.T) {
    defer gock.off()

    snapshot := gocksnap.MatchSnapshot(t, "name of the scenario")

    // put you code here that calls HTTP endpoint
    // ...
    
    // call finish to save the snapshot and assert that the test is complete
    snapshot.Finish(t)
}
```

## Preview

When running the test for the first time, the library will open a web server to select the response for each request:

<img width="1040" height="871" alt="image1" src="https://github.com/user-attachments/assets/fb61570d-f308-4617-b14f-b727085fb73b" />

Select the desired `status` and `response body` that should be returned for this request, and click `Save Snapshot`.

## Updating a snapshot

To edit an existing snapshot, you can run your test with the environment variable `UPDATE_GOCKSNAP=true`, a new button will appear to reuse the response from the existing snapshot:

<img width="1039" height="334" alt="image2" src="https://github.com/user-attachments/assets/53d3ca42-7e0f-4a61-b319-e5997ef95edc" />
