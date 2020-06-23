
protoc.exe proto\*.proto -I proto -I third_party -I %GOPATH%\src\github.com\protocolbuffers\protobuf\src --go_out=plugins=grpc:. --grpc-gateway_out=logtostderr=true,allow_delete_body=true:. --swagger_out=logtostderr=true,allow_delete_body=true:.

move cdb.swagger.json swagger\
