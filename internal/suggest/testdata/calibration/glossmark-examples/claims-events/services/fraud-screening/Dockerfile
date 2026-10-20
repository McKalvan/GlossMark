FROM gradle:8.10-jdk21 AS build
WORKDIR /src
COPY schemas /schemas
COPY services/fraud-screening /src
RUN gradle --no-daemon installDist

FROM eclipse-temurin:21-jre-alpine
RUN addgroup -S fraud && adduser -S fraud -G fraud
COPY --from=build /src/build/install/fraud-screening /opt/fraud-screening
USER fraud
ENTRYPOINT ["/opt/fraud-screening/bin/fraud-screening"]
