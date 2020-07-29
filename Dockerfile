FROM golang as builder

WORKDIR /go/src/microservice
COPY . .
RUN apt-get update
RUN CGO_ENABLED=0 go build -v

FROM alpine
LABEL maintainer="jayaraj.esvar@gmail.com"
WORKDIR /home
COPY --from=builder /go/src/microservice/go-websocket /home
COPY --from=builder /go/src/microservice/conf /home/conf
COPY --from=builder /go/src/microservice/public /home/public
CMD [ "/home/go-websocket" ]
