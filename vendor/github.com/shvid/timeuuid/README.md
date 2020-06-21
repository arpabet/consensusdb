# timeuuid
Golang UUID implementation that supports TimeUUID version

### Checkout
```
go get "github.com/consensusdb/timeuuid"
```

### Import
```
import "github.com/consensusdb/timeuuid"
```

### Quick start example:
```
	uuid := timeuuid.NewUUID(timeuuid.TimebasedUUID)
	uuid.SetUnixTimeMillis(123)
	uuid.SetCounter(555)
	fmt.Print(uuid.MarshalBinary())
	uuid.Parse(uuid.String())
```