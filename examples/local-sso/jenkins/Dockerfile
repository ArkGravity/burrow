FROM jenkins/jenkins:2.580.1-jdk21@sha256:a660310e39ade10631f774bacd5219de767dfd08947ea5c32a739b5e5bb382c1

COPY --chown=jenkins:jenkins plugins.txt /usr/share/jenkins/ref/plugins.txt
COPY --chown=jenkins:jenkins verify-plugins.sh /usr/local/bin/verify-plugins
RUN jenkins-plugin-cli --jenkins-update-center https://updates.jenkins.io/update-center.actual.json \
        --jenkins-update-center-download-url https://archives.jenkins.io/plugins \
        --plugin-file /usr/share/jenkins/ref/plugins.txt --latest false \
    && sh /usr/local/bin/verify-plugins

USER jenkins
