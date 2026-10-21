// Isolated test fixture only. The local-sso example is configured manually.
import groovy.json.JsonSlurper
import hudson.security.GlobalMatrixAuthorizationStrategy
import jenkins.model.Jenkins
import jenkins.model.JenkinsLocationConfiguration
import org.jenkinsci.plugins.matrixauth.AuthorizationType
import org.jenkinsci.plugins.matrixauth.PermissionEntry
import org.jenkinsci.plugins.saml.IdpMetadataConfiguration
import org.jenkinsci.plugins.saml.SamlAdvancedConfiguration
import org.jenkinsci.plugins.saml.SamlEncryptionData
import org.jenkinsci.plugins.saml.SamlSecurityRealm

def fixture = new JsonSlurper().parse(new File('/fixture/users.json'))
def jenkins = Jenkins.get()
jenkins.setNumExecutors(0)
JenkinsLocationConfiguration.get().setUrl(System.getenv('JENKINS_URL') + '/')
def strategy = new GlobalMatrixAuthorizationStrategy()
strategy.add(Jenkins.ADMINISTER, new PermissionEntry(AuthorizationType.USER, fixture.adminId))
strategy.add(Jenkins.READ, new PermissionEntry(AuthorizationType.USER, fixture.readerId))
jenkins.setAuthorizationStrategy(strategy)
def advanced = new SamlAdvancedConfiguration(false, null, null,
    'urn:oasis:names:tc:SAML:2.0:nameid-format:persistent')
def encryption = new SamlEncryptionData(null, null, null, null, true, true)
def metadata = new IdpMetadataConfiguration(new File('/fixture/idp.xml').getText('UTF-8'))
jenkins.setSecurityRealm(new SamlSecurityRealm(metadata, 'displayName', '', 86400,
    null, 'email', null, advanced, encryption, null,
    'urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect', []))
jenkins.save()
