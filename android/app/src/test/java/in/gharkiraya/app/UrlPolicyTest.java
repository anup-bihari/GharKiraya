package in.gharkiraya.app;

import org.junit.Test;
import static org.junit.Assert.*;

public final class UrlPolicyTest {
    private static final String HOME = "https://gharkiraya-ten.vercel.app/";
    @Test public void acceptsWebsitePagesFragmentsAndHttpsPort() {
        assertTrue(UrlPolicy.isWebsite(HOME, HOME));
        assertTrue(UrlPolicy.isWebsite(HOME, HOME + "#messages"));
        assertTrue(UrlPolicy.isWebsite(HOME, "https://GHARKIRAYA-TEN.VERCEL.APP:443/api/me"));
    }
    @Test public void rejectsLookalikesExternalOriginsAndCredentials() {
        assertFalse(UrlPolicy.isWebsite(HOME, "https://gharkiraya-ten.vercel.app.evil.example/"));
        assertFalse(UrlPolicy.isWebsite(HOME, "https://gharkiraya-ten.vercel.app@evil.example/"));
        assertFalse(UrlPolicy.isWebsite(HOME, "https://user@gharkiraya-ten.vercel.app/"));
        assertFalse(UrlPolicy.isWebsite(HOME, "https://gharkiraya-ten.vercel.app:444/"));
    }
    @Test public void rejectsUntrustedSchemesAndMalformedUrls() {
        for (String url : new String[]{"http://gharkiraya-ten.vercel.app/", "javascript:alert(1)",
                "file:///etc/passwd", "content://downloads/1", "intent://example", "//gharkiraya-ten.vercel.app/", "https://[", "", null}) {
            assertFalse(UrlPolicy.isWebsite(HOME, url));
        }
    }
}
