# Abastevo license

Current project-owned material is offered under the
[Abastevo Visible Attribution License 1.0](../LICENSE)
(`LicenseRef-Abastevo-Visible-Attribution-1.0`). The maintainer requested the
same terms as the [TBG license](https://github.com/AlexandreZanata/TBG-Tupi-Battle-Grounds/blob/main/LICENSE),
adapted from a game to the Abastevo fuel system.

Use, modification, hosting and redistribution are permitted for any purpose,
including commercial use, without royalties or a source-publication requirement
for material covered by this license. The complete license controls those rights.
This is a custom license, not the MIT license or a claim of OSI approval.

## Original owner and official APK

The original copyright holder's own distributions are exempt from visible
self-credit. The official Abastevo APK keeps its existing interface without an
author-credit strip. The certificate, license file and third-party notices are
still preserved; these are legal/signing records, not a forced on-screen footer.
The exemption does not extend to independent users, hosts or derivative projects.

## Visible credit for third parties

Third-party normal on-screen interfaces must keep readable project and creator credit,
including during map/navigation use:

- Original system: **Abastevo**, linked to
  [the original repository](https://github.com/AlexandreZanata/Abastevo).
- Creator: **Alexandre Zanata (@AlexandreZanata)**, linked to
  [the creator's GitHub](https://github.com/AlexandreZanata).

Both destinations must be clearly labeled clickable links. When an interface
cannot offer clickable links, show their full URLs as readable text instead.
Credit may be translated or restyled while preserving both identities and
original destinations. An optional About screen, source comment, documentation
or first-launch acknowledgment alone does not satisfy the visible-credit rule.
Keep the license/copyright with redistributed copies and substantial portions.
Do not imply the creator endorses a derivative work or business.

## Original and third-party notices

The [original MIT notice](../LICENSES/MIT-original.txt) remains preserved.
Historical revisions retain the license accompanying those revisions; this
change does not rewrite old release artifacts. Third-party components and data
retain their own terms, including the Apache-2.0 CoMaps SDK and its packaged
MIT/BSD/other notices. See [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)
and [data attribution](../README.md#data-attribution).

The material remains supplied without warranty, as described in the full license.

## Example for third-party interfaces

1. Put a persistent, readable credit in your normal application interface. Reserve
   layout space so it does not cover controls; support large text and contrast.
2. Identify the original project and creator, with separate labeled clickable
   links to the exact destinations below. Keep the credit during map/navigation
   and other normal use; do not hide it in About, docs or first launch.
3. Ship the complete LICENSE, copyright and applicable third-party notices.
4. Keep both identities/destinations when translating/restyling. If your interface
   cannot open links, show the full URLs as readable text instead.

An HTML example (adapt the layout and equivalent native controls to your app):

```html
<aside class="original-project-credit" aria-label="Original project and creator">
  <a href="https://github.com/AlexandreZanata/Abastevo">
    Original project: Abastevo — GitHub
  </a>
  <a href="https://github.com/AlexandreZanata">
    Creator: Alexandre Zanata (@AlexandreZanata) — GitHub
  </a>
</aside>
```

The snippet supplies the required identities and links. Your layout must keep
them readable and visible during normal use; placing it at the end of a long
page without persistent visibility does not meet that condition. On Android,
use accessible link buttons in a measured persistent footer and reserve its
actual height in the main Scaffold so navigation controls remain usable.

For a third-party Android app using Compose/Material 3, the same requirement
can be implemented in the root Scaffold (this is documentation, not a footer
in the official owner's APK):

```kotlin
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler

@Composable
fun ThirdPartyAppWithCredit(content: @Composable () -> Unit) {
    val links = LocalUriHandler.current
    Scaffold(
        bottomBar = {
            Column(Modifier.navigationBarsPadding()) {
                TextButton(onClick = {
                    links.openUri("https://github.com/AlexandreZanata/Abastevo")
                }) {
                    Text("Original project: Abastevo — GitHub")
                }
                TextButton(onClick = {
                    links.openUri("https://github.com/AlexandreZanata")
                }) {
                    Text("Creator: Alexandre Zanata (@AlexandreZanata) — GitHub")
                }
            }
        },
    ) { insets ->
        Box(Modifier.fillMaxSize().padding(insets)) { content() }
    }
}
```

Integrate your normal bottom navigation into the same measured footer when
needed. Test small screens, large text, contrast, independent link activation
and active navigation; do not put a covering overlay over this credit. Translate
the labels if needed, preserving Abastevo, Alexandre Zanata and both URLs.

For a web interface, reserve the measured footer height rather than covering
content. For example, combine the HTML above with:

```css
body { padding-bottom: var(--original-credit-height, 10rem); }
.original-project-credit {
  position: fixed; inset: auto 0 0; z-index: 90;
  display: flex; flex-wrap: wrap; justify-content: center; gap: .5rem 1.5rem;
  padding: .5rem 1rem calc(.5rem + env(safe-area-inset-bottom, 0px));
  background: #fff; color: #111; border-top: 1px solid #777;
}
.original-project-credit a {
  display: inline-flex; align-items: center; min-height: 48px;
  max-width: 100%; overflow-wrap: anywhere; color: #0645ad;
}
.original-project-credit a:focus-visible { outline: 3px solid #0645ad; }
```

```js
const credit = document.querySelector('.original-project-credit');
const reserveCreditSpace = () => document.documentElement.style.setProperty(
  '--original-credit-height', `${Math.ceil(credit.getBoundingClientRect().height)}px`,
);
new ResizeObserver(reserveCreditSpace).observe(credit);
reserveCreditSpace();
```

Use equivalent colors for your theme with readable contrast, and verify that
your other fixed controls also respect this reserved area. This example performs
no API request and does not inject a footer into the official Abastevo build.
