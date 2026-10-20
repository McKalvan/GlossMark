FROM gradle:8.10-jdk21 AS build
WORKDIR /workspace
COPY settings.gradle.kts build.gradle.kts gradle.properties ./
COPY service/build.gradle.kts service/
RUN gradle :service:dependencies --no-daemon -q > /dev/null
COPY service/src service/src
RUN gradle :service:bootJar --no-daemon -x test

FROM eclipse-temurin:21-jre
RUN useradd --system --uid 10001 shipment
WORKDIR /app
COPY --from=build /workspace/service/build/libs/shipment-core.jar app.jar
USER shipment
EXPOSE 8080
ENTRYPOINT ["java", "-XX:MaxRAMPercentage=75", "-jar", "/app/app.jar"]
