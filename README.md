# bigbagger

BigBagger Database

This project started on Halloween! Ahahahahahahaha

# Quick start

### Build
```
go build
```

Build for Linux Amd64
```
env GOOS=linux GOARCH=amd64 go build
```

### Open

```
open http://localhost:4481/
```


### Run
```
./bigbagger
curl -d "@create.json" -H "Content-Type: application/json" -X POST http://localhost:4481/v1/dataset
```

### Check

```
lsof -n -i:$PORT | grep LISTEN
```

You have to see that bigbagger is listening 4481 and 4482 ports

### Go Client Example

```
    //
    // connect to BigBagger server
    //

    client, err := bbclient.NewClient(grpcAddress)
	defer client.Close()

    //
    // create dataset TEST
    //

	dataset := new(bbproto.Dataset)
	dataset.Version = "1.0"
	dataset.Name = "TEST"

	err = client.CreateDataset(dataset)

    //
    // Put
    //

	op = bbclient.Put(set, []byte("key"), []byte("value"))

	res, err = client.Execute(op)

    //
    // Get
    //

	op = bbclient.Get(set, []byte("key"))

	res, err = client.Execute(op)

	if res.IsError() {
		fmt.Print("get error: ", res.GetError())
		return
	}

	data := res.GetValue()

```

