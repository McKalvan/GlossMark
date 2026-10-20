FROM gradle:8.10-jdk21 AS build
WORKDIR /src
COPY schemas /schemas
COPY services/claim-intake /src
RUN gradle --no-daemon installDist

FROM eclipse-temurin:21-jre-alpine
RUN addgroup -S intake && adduser -S intake -G intake
COPY --from=build /src/build/install/claim-intake /opt/claim-intake
USER intake
EXPOSE 8080
ENTRYPOINT ["/opt/claim-intake/bin/claim-intake"]
