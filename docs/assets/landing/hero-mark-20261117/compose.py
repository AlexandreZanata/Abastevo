"""Compose the home hero art: the approved brand mark (verbatim vector) on a
map-like field, with the mark's green ribbon continued as a route."""
import re, sys
from pathlib import Path
root, out = Path(sys.argv[1]), Path(sys.argv[2])
logo = (root / 'docs/assets/brand/abastevo-logo.svg').read_text(encoding='utf-8')
inner = logo[logo.index('<defs>'):logo.rindex('</svg>')].strip()
# prefix ids so the mark's defs never collide with the art's own
ids = re.findall(r'id="([^"]+)"', inner)
for i in ids:
    inner = inner.replace(f'id="{i}"', f'id="m-{i}"').replace(f'url(#{i})', f'url(#m-{i})')
assert 'href' not in inner and '<image' not in inner

def pin(x, y, fill, s=1.0):
    return (f'<g transform="translate({x} {y}) scale({s})"><path d="M0 0C-22-30-38-50-38-74a38 38 0 0 1 76 0C38-50 22-30 0 0Z" fill="{fill}"/>'
            f'<circle cx="0" cy="-74" r="14" fill="#fff"/></g>')

def card(x, y, w, h, body):
    return (f'<g transform="translate({x} {y})"><rect x="6" y="10" width="{w}" height="{h}" rx="26" fill="#000" opacity=".07"/>'
            f'<rect width="{w}" height="{h}" rx="26" fill="#fff" stroke="#e2e2e2" stroke-width="2"/>{body}</g>')

price_card = card(70, 610, 300, 170,
    '<circle cx="50" cy="52" r="22" fill="#e6f4ee"/><path d="M50 38c8 10 12 16 12 22a12 12 0 0 1-24 0c0-6 4-12 12-22Z" fill="#007a4b"/>'
    '<rect x="88" y="36" width="130" height="14" rx="7" fill="#000"/><rect x="88" y="60" width="86" height="10" rx="5" fill="#c9c9c9"/>'
    '<rect x="28" y="104" width="150" height="30" rx="10" fill="#000"/><rect x="194" y="108" width="78" height="22" rx="11" fill="#e6f4ee"/>'
    '<circle cx="210" cy="119" r="5" fill="#007a4b"/><rect x="222" y="115" width="38" height="8" rx="4" fill="#007a4b"/>')
chart_card = card(560, 150, 270, 190,
    '<rect x="28" y="28" width="110" height="14" rx="7" fill="#000"/><rect x="28" y="52" width="70" height="10" rx="5" fill="#c9c9c9"/>'
    '<rect x="30" y="118" width="38" height="44" rx="8" fill="#d9d9d9"/><rect x="82" y="98" width="38" height="64" rx="8" fill="#d9d9d9"/>'
    '<rect x="134" y="108" width="38" height="54" rx="8" fill="#d9d9d9"/><rect x="186" y="84" width="38" height="78" rx="8" fill="#0052f7"/>')
svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 1000" width="900" height="1000" preserveAspectRatio="xMidYMid slice">
<defs>
<linearGradient id="h-green" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#03664b"/><stop offset=".55" stop-color="#05a579"/><stop offset="1" stop-color="#33e596"/></linearGradient>
<linearGradient id="h-blue" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#0166f8"/><stop offset="1" stop-color="#002284"/></linearGradient>
<pattern id="h-dots" width="36" height="36" patternUnits="userSpaceOnUse"><circle cx="2" cy="2" r="2" fill="#d6d6d6"/></pattern>
</defs>
<rect width="900" height="1000" fill="#f3f3f3"/>
<rect width="900" height="1000" fill="url(#h-dots)"/>
<g fill="none" stroke="#e2e2e2" stroke-width="2"><circle cx="450" cy="470" r="300"/><circle cx="450" cy="470" r="420"/><circle cx="450" cy="470" r="540"/></g>
<path d="M-60 300C140 250 250 420 430 400S700 180 960 250" fill="none" stroke="url(#h-blue)" stroke-width="56" stroke-linecap="round"/>
<path d="M-60 300C140 250 250 420 430 400S700 180 960 250" fill="none" stroke="#fff" stroke-width="4" stroke-dasharray="22 26" opacity=".7"/>
<path d="M-60 920C200 940 300 720 470 700S740 820 960 640" fill="none" stroke="url(#h-green)" stroke-width="72" stroke-linecap="round"/>
<path d="M-60 920C200 940 300 720 470 700S740 820 960 640" fill="none" stroke="#fff" stroke-width="5" stroke-dasharray="26 30" opacity=".8"/>
<circle cx="450" cy="470" r="226" fill="#000" opacity=".06" transform="translate(8 14)"/>
<circle cx="450" cy="470" r="226" fill="#fff" stroke="#e2e2e2" stroke-width="2"/>
<g transform="translate(252.5 277.2) scale(0.3125)">{inner}</g>
{chart_card}
{price_card}
{pin(690, 700, '#000')}
{pin(250, 330, '#000', .8)}
{pin(800, 420, '#007a4b', .7)}
</svg>
'''
out.write_text(svg, encoding='utf-8')
