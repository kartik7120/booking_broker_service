FROM alpine:latest

WORKDIR /app

COPY ./brokerApp /app/brokerApp

CMD ["./brokerApp"]