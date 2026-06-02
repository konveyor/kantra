########################################
# Build Image
########################################
# FROM maven:3.6-jdk-8-slim as build
FROM maven:3.8-openjdk-11 as build

WORKDIR /app

# Establish the dependency layer
COPY pom.xml .
RUN mvn dependency:resolve

# Add the source code and package
COPY src ./src
RUN mvn package

########################################
# Production Image
########################################
# FROM tomcat:9-jdk8-openjdk-slim
FROM tomcat:9-jdk11-openjdk-slim

COPY --from=build --chown=1001:0 /app/target/customers-tomcat-0.0.1-SNAPSHOT.war /usr/local/tomcat/webapps/ROOT.war

EXPOSE 8080
CMD ["catalina.sh", "run"]
