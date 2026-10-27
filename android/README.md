# GharKiraya for Android

This Android app displays **https://gharkiraya-ten.vercel.app/** in Android WebView. The live website supplies every screen, style, listing, account, and rental workflow. Website changes appear in the app without rebuilding. This is a website-backed Android client, not a separately implemented native interface; it requires internet access and a working website.

## Build and install

Open this `android` directory in Android Studio. Install JDK 21, Android SDK Platform 36, and Build Tools 36.0.0, then sync the project.

```sh
./gradlew assembleDebug lintDebug testDebugUnitTest
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

The debug APK is for development. Create your own release signing key in Android Studio's **Generate Signed Bundle / APK** flow before distribution. Signing keys and local SDK paths must never be committed.

## Behavior

- Uses the website's mobile layout, branding, fonts, and features directly.
- Retains first-party cookies and local storage for sign-in.
- Android back navigates website history and exits at the root.
- Displays loading progress and a retry screen for connection failures.
- Opens external HTTPS, email, and telephone links in an appropriate external app.
- Supports the repository frontend's text agreement Blob downloads through Android's Save document picker.
- Keeps HTTPS certificate validation enabled and blocks cleartext/mixed content, file access, and third-party cookies.
- Only debug builds expose WebView inspection through Chrome.

Update `app/src/main/res/values/strings.xml` to change the website URL. No backend credentials are embedded. The Go server at the repository root is a separate development backend and is not needed when this app connects to the hosted website.

## Device acceptance checks

On a physical device or emulator with a current Android System WebView:

1. Compare the homepage, spacing, cards, images, dialogs, and navigation with the same website in mobile Chrome.
2. Exercise search and filters, property details, registration/sign-in, saved homes, tenant requirements, and landlord property listing.
3. With tenant and landlord accounts, send/accept an interest request, exchange messages, create and acknowledge an agreement, and save its text file.
4. Rotate the phone, background/reopen the app, and check session persistence, keyboard behavior, and back navigation.
5. Disable networking, reload, and confirm that the retry screen recovers after connectivity returns.
6. Check external links and the document picker, including cancellation.

Build/lint success does not replace these device checks or prove availability of the hosted website.
