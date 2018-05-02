#
# Run this file with: docker run --rm -v $(pwd):/workspace nocquidant/sconfe
#
# Multi stage build
#  - We don't need go installed once our app is compiled
#  - Leaving the build image

FROM golang:1.10 AS build
ADD . /src
WORKDIR /src
RUN go get -d -v -t 
RUN go test --cover -v ./... --run UnitTest 
RUN go build -v -o sconfe

# ---

FROM alpine:3.7 

# https://stackoverflow.com/questions/34729748/installed-go-binary-not-found-in-path-on-alpine-linux-docker
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2   

VOLUME /workspace

ENTRYPOINT ["sconfe"]
CMD [""]

COPY --from=build /src/sconfe /usr/local/bin/sconfe

RUN chmod +x /usr/local/bin/sconfe