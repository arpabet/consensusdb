#!/bin/bash

protoc proto/*.proto -I proto --go_out=plugins=grpc:. --grpc-gateway_out=logtostderr=true,allow_delete_body=true:. --swagger_out=logtostderr=true,allow_delete_body=true:.

cp cserverpb.swagger.json 3rdparty/swagger-ui/
