package in.gharkiraya.app;

import java.net.URI;
import java.net.URISyntaxException;

/** Restricts embedded navigation and authenticated downloads to the configured HTTPS origin. */
final class UrlPolicy {
    static boolean isWebsite(String home, String candidate) {
        if (home == null || candidate == null) return false;
        try {
            URI expected = new URI(home);
            URI actual = new URI(candidate);
            return "https".equalsIgnoreCase(expected.getScheme())
                    && "https".equalsIgnoreCase(actual.getScheme())
                    && expected.getHost() != null
                    && expected.getHost().equalsIgnoreCase(actual.getHost())
                    && (actual.getPort() == -1 || actual.getPort() == 443)
                    && actual.getRawUserInfo() == null;
        } catch (URISyntaxException e) {
            return false;
        }
    }
    private UrlPolicy() {}
}
