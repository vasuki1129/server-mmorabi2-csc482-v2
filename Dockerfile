FROM public.ecr.aws/docker/library/golang:latest AS build

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o main .

FROM public.ecr.aws/docker/library/alpine:latest

RUN apk update && \
    apk upgrade && \
    apk add ca-certificates && \
    apk add tzdata

WORKDIR /app

COPY --from=build /build/main ./

RUN pwd && find .

EXPOSE 8080

CMD ["./main"]