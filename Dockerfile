FROM golang:1.21-bullseye

RUN apt-get update && apt-get install -y ffmpeg && apt-get clean

WORKDIR /app

COPY . .

RUN go build -o transcoder main.go

EXPOSE 8080

CMD ["./transcoder"]
