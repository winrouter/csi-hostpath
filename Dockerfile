FROM golang:1.20.4 AS builder

WORKDIR /go/src/github.com/winrouter/csi-hostpath
COPY . .
RUN make build && chmod +x bin/csi-hostpath

FROM alpine:3.9
LABEL maintainers="archeros Cloud Authors"
LABEL description="csi-hostpath is a local disk management system"
RUN apk update && apk upgrade && apk add util-linux coreutils e2fsprogs e2fsprogs-extra xfsprogs xfsprogs-extra blkid file open-iscsi jq
COPY --from=builder /go/src/github.com/winrouter/csi-hostpath/bin/csi-hostpath /bin/csi-hostpath
ENTRYPOINT ["csi-hostpath"]