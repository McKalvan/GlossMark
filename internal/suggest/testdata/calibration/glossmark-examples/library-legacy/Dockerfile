# @gloss deploys .CircWebApp, .NightlyBatch scope=file conf=0.85
# Added 2024 for the cloud move. Before that, build.xml's "deploy" target
# copied circ.war onto circ-app01's Tomcat. The image also carries the batch
# jobs so the nightly ECS task can reuse it.
FROM tomcat:9.0-jdk11-temurin

RUN apt-get update \
 && apt-get install -y --no-install-recommends perl libdbd-pg-perl libyaml-tiny-perl libmime-lite-perl ruby ruby-pg postgresql-client \
 && rm -rf /var/lib/apt/lists/*

COPY build/circ.war /usr/local/tomcat/webapps/ROOT.war
COPY batch /opt/circ/batch
COPY config /etc/circ

ENV CIRC_CONFIG_DIR=/etc/circ
EXPOSE 8080
CMD ["catalina.sh", "run"]
