package in.gharkiraya.app;

import android.annotation.SuppressLint;
import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.util.Base64;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.CookieManager;
import android.webkit.JsPromptResult;
import android.webkit.SslErrorHandler;
import android.webkit.URLUtil;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.net.http.SslError;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.OutputStream;

/** The production website is the single source of UI, data, and account behavior. */
public final class MainActivity extends ComponentActivity {
    private static final int SAVE_DOCUMENT = 41;
    private static final String DOWNLOAD_PROMPT = "gharkiraya-download:";
    private WebView web;
    private ProgressBar progress;
    private LinearLayout errorPanel;
    private TextView errorText;
    private String home;
    private boolean failed;
    private byte[] pendingDownload;

    @Override public void onCreate(Bundle savedState) {
        super.onCreate(savedState);
        home = getString(R.string.website_url);
        Object retained = getLastCustomNonConfigurationInstance();
        if (retained instanceof byte[]) pendingDownload = (byte[]) retained;
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.rgb(255, 253, 246));
        if (Build.VERSION.SDK_INT >= 30) {
            root.setOnApplyWindowInsetsListener((v, insets) -> {
                // Keep the website and keyboard clear of system bars on edge-to-edge Android.
                android.graphics.Insets bars = insets.getInsets(
                        android.view.WindowInsets.Type.systemBars() | android.view.WindowInsets.Type.ime());
                v.setPadding(bars.left, bars.top, bars.right, bars.bottom);
                return insets;
            });
        } else {
            root.setOnApplyWindowInsetsListener((v, insets) -> {
                v.setPadding(insets.getSystemWindowInsetLeft(), insets.getSystemWindowInsetTop(),
                        insets.getSystemWindowInsetRight(), insets.getSystemWindowInsetBottom());
                return insets;
            });
        }
        web = new WebView(this);
        web.setBackgroundColor(Color.rgb(255, 253, 246));
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));
        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        progress.setMax(100);
        root.addView(progress, new FrameLayout.LayoutParams(-1, dp(3), Gravity.TOP));
        createErrorPanel(root);
        setContentView(root);
        configureWebView();
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override public void handleOnBackPressed() { navigateBack(); }
        });
        if (savedState == null || web.restoreState(savedState) == null) web.loadUrl(home);
    }

    // The first-party application requires JavaScript; restrict navigation and native handoffs to its HTTPS origin.
    @SuppressLint("SetJavaScriptEnabled")
    private void configureWebView() {
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(false);
        settings.setJavaScriptCanOpenWindowsAutomatically(false);
        settings.setMediaPlaybackRequiresUserGesture(true);
        CookieManager.getInstance().setAcceptCookie(true);
        CookieManager.getInstance().setAcceptThirdPartyCookies(web, false);
        WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG);
        web.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                if (!request.isForMainFrame()) return false;
                if (isWebsite(request.getUrl())) return false;
                openExternal(request.getUrl());
                return true;
            }
            @Override public void onPageStarted(WebView view, String url, android.graphics.Bitmap icon) {
                failed = false;
                errorPanel.setVisibility(View.GONE);
                progress.setVisibility(View.VISIBLE);
            }
            @Override public void onPageFinished(WebView view, String url) {
                progress.setVisibility(View.GONE);
                CookieManager.getInstance().flush();
                if (!failed) web.setVisibility(View.VISIBLE);
            }
            @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) showError("Check your internet connection, then try again.");
            }
            @Override public void onReceivedHttpError(WebView view, WebResourceRequest request, WebResourceResponse response) {
                if (request.isForMainFrame()) showError("The website is temporarily unavailable. Please try again shortly.");
            }
            @Override public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                handler.cancel(); // Never bypass certificate validation.
                if (error.getUrl().equals(view.getUrl()))
                    showError("A secure connection could not be established. Please try again later.");
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override public void onProgressChanged(WebView view, int value) {
                progress.setProgress(value);
                progress.setVisibility(value == 100 || failed ? View.GONE : View.VISIBLE);
            }
            @Override public boolean onJsPrompt(WebView view, String url, String message, String defaultValue, JsPromptResult result) {
                // Scoped, text-only download handoff. No general-purpose JavaScript/native bridge.
                if (isWebsite(Uri.parse(url)) && isWebsite(Uri.parse(view.getUrl() == null ? "" : view.getUrl()))
                        && message != null && message.startsWith(DOWNLOAD_PROMPT)) {
                    result.confirm("");
                    try {
                        if (pendingDownload != null) throw new IllegalStateException("Save already in progress");
                        String encoded = message.substring(DOWNLOAD_PROMPT.length());
                        if (encoded.length() > 2_000_000) throw new IllegalArgumentException("File too large");
                        pendingDownload = Base64.decode(encoded, Base64.DEFAULT);
                        Intent save = new Intent(Intent.ACTION_CREATE_DOCUMENT);
                        save.addCategory(Intent.CATEGORY_OPENABLE);
                        save.setType("text/plain");
                        save.putExtra(Intent.EXTRA_TITLE, "GharKiraya-agreement.txt");
                        startActivityForResult(save, SAVE_DOCUMENT);
                    } catch (Exception e) {
                        pendingDownload = null;
                        Toast.makeText(MainActivity.this, "Could not save the agreement. Please try again.", Toast.LENGTH_LONG).show();
                    }
                    return true;
                }
                result.cancel();
                return true;
            }
        });
        web.setDownloadListener((url, userAgent, disposition, mime, length) -> {
            if (!isWebsite(Uri.parse(web.getUrl() == null ? "" : web.getUrl()))) return;
            if (url.startsWith("blob:")) {
                // Website agreements use Blob URLs, which Android DownloadManager cannot read.
                web.evaluateJavascript("(async()=>{try{const r=await fetch(" + JSONObject.quote(url)
                        + ");const b=await r.blob();if(b.size>1000000||!b.type.startsWith('text/plain'))throw Error();"
                        + "const f=new FileReader();f.onload=()=>prompt('" + DOWNLOAD_PROMPT
                        + "'+f.result.split(',')[1],'');f.readAsDataURL(b);"
                        + "}catch(e){alert('Could not download this file. Please try again.')}})()", null);
            } else if (isWebsite(Uri.parse(url))) {
                try {
                    DownloadManager.Request request = new DownloadManager.Request(Uri.parse(url));
                    request.addRequestHeader("User-Agent", userAgent);
                    String cookies = CookieManager.getInstance().getCookie(url);
                    if (cookies != null) request.addRequestHeader("Cookie", cookies);
                    request.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
                    request.setDestinationInExternalFilesDir(this, Environment.DIRECTORY_DOWNLOADS,
                            URLUtil.guessFileName(url, disposition, mime));
                    ((DownloadManager) getSystemService(DOWNLOAD_SERVICE)).enqueue(request);
                    Toast.makeText(this, "Download started", Toast.LENGTH_SHORT).show();
                } catch (RuntimeException e) {
                    Toast.makeText(this, "Download unavailable. Please try again.", Toast.LENGTH_LONG).show();
                }
            }
        });
    }

    private boolean isWebsite(Uri uri) {
        return UrlPolicy.isWebsite(home, uri.toString());
    }

    private void openExternal(Uri uri) {
        String scheme = uri.getScheme();
        if (!("https".equalsIgnoreCase(scheme) || "mailto".equalsIgnoreCase(scheme) || "tel".equalsIgnoreCase(scheme))) return;
        try { startActivity(new Intent(Intent.ACTION_VIEW, uri)); }
        catch (ActivityNotFoundException e) { Toast.makeText(this, "No app available to open this link", Toast.LENGTH_SHORT).show(); }
    }

    private void createErrorPanel(FrameLayout root) {
        errorPanel = new LinearLayout(this);
        errorPanel.setOrientation(LinearLayout.VERTICAL);
        errorPanel.setGravity(Gravity.CENTER);
        errorPanel.setPadding(dp(32), dp(32), dp(32), dp(32));
        errorPanel.setBackgroundColor(Color.rgb(255, 253, 246));
        TextView title = new TextView(this);
        title.setText(R.string.connection_title);
        title.setTextSize(26);
        title.setTextColor(Color.rgb(35, 78, 62));
        title.setGravity(Gravity.CENTER);
        errorPanel.addView(title);
        errorText = new TextView(this);
        errorText.setTextSize(16);
        errorText.setGravity(Gravity.CENTER);
        errorText.setPadding(0, dp(16), 0, dp(24));
        errorPanel.addView(errorText);
        Button retry = new Button(this);
        retry.setText(R.string.retry);
        retry.setOnClickListener(v -> web.loadUrl(isWebsite(Uri.parse(web.getUrl() == null ? "" : web.getUrl())) ? web.getUrl() : home));
        errorPanel.addView(retry);
        errorPanel.setVisibility(View.GONE);
        root.addView(errorPanel, new FrameLayout.LayoutParams(-1, -1));
    }

    private void showError(String message) {
        failed = true;
        progress.setVisibility(View.GONE);
        web.setVisibility(View.INVISIBLE);
        errorText.setText(message);
        errorPanel.setVisibility(View.VISIBLE);
    }

    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }
    private void navigateBack() { if (web.canGoBack()) web.goBack(); else finish(); }
    @Override public Object onRetainCustomNonConfigurationInstance() { return pendingDownload; }
    @Override protected void onSaveInstanceState(Bundle out) { web.saveState(out); super.onSaveInstanceState(out); }
    @Override protected void onPause() { CookieManager.getInstance().flush(); web.onPause(); super.onPause(); }
    @Override protected void onResume() { super.onResume(); if (web != null) web.onResume(); }
    @Override protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != SAVE_DOCUMENT) return;
        byte[] bytes = pendingDownload;
        pendingDownload = null;
        if (resultCode != RESULT_OK || data == null || data.getData() == null || bytes == null) return;
        try (OutputStream output = getContentResolver().openOutputStream(data.getData())) {
            if (output == null) throw new java.io.IOException("No document stream");
            output.write(bytes);
            Toast.makeText(this, "Agreement saved", Toast.LENGTH_SHORT).show();
        } catch (Exception e) { Toast.makeText(this, "Could not save the agreement", Toast.LENGTH_LONG).show(); }
    }
    @Override protected void onDestroy() {
        if (web != null) { ((ViewGroup) web.getParent()).removeView(web); web.destroy(); }
        super.onDestroy();
    }
}
