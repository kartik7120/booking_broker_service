FROM alpine:latest

WORKDIR /app

COPY brokerApp /app/brokerApp

RUN chmod +x brokerApp

CMD ["./brokerApp"]