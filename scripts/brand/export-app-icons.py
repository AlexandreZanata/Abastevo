#!/usr/bin/env python3
"""Export native launcher assets deterministically from the approved SVG master.
Requires Inkscape and Pillow (asset tooling only, not app dependencies).
"""
from pathlib import Path
import copy, hashlib, json, subprocess, tempfile, xml.etree.ElementTree as ET
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
BRAND = ROOT / 'docs/assets/brand'
RES = ROOT / 'app/src/main/res'
IOS = ROOT / 'iosApp/Assets.xcassets/AppIcon.appiconset'
SVG_NS = 'http://www.w3.org/2000/svg'
ANDROID_NS = 'http://schemas.android.com/apk/res/android'
ET.register_namespace('', SVG_NS)
S = lambda name: '{' + SVG_NS + '}' + name
A = lambda name: '{' + ANDROID_NS + '}' + name

def render(source, output, size):
    subprocess.run(['inkscape', str(source), '--export-type=png',
                    f'--export-width={size}', f'--export-filename={output}'],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

def save_xml(root, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    ET.indent(root)
    ET.ElementTree(root).write(path, encoding='utf-8', xml_declaration=True)

master_path = BRAND / 'abastevo-logo.svg'
master_hash = hashlib.sha256(master_path.read_bytes()).hexdigest()
master = ET.parse(master_path).getroot()
with tempfile.TemporaryDirectory(prefix='abastevo-icons-') as directory:
    tmp = Path(directory)
    render(master_path, tmp / 'mark.png', 1254)
    alpha = Image.open(tmp / 'mark.png').getchannel('A')
    bbox = alpha.point(lambda value: 255 if value > 128 else 0).getbbox()
    left, top, right, bottom = bbox
    cx, cy = (left + right) / 2, (top + bottom) / 2
    pixels = alpha.load()
    radius = max(((x - cx)**2 + (y - cy)**2)**0.5
                 for y in range(top, bottom) for x in range(left, right)
                 if pixels[x, y] > 0)
    # Fit the full silhouette inside the 66dp circular adaptive safe zone.
    adaptive_scale = 32.5 / radius
    ax, ay = 54 - cx * adaptive_scale, 54 - cy * adaptive_scale
    icon_scale = 760 / (right - left)
    ix, iy = 512 - cx * icon_scale, 512 - cy * icon_scale
    def svg_asset(size, scale, tx, ty, white):
        root = ET.Element(S('svg'), {'width':str(size), 'height':str(size),
                          'viewBox':f'0 0 {size} {size}', 'role':'img'})
        ET.SubElement(root,S('title')).text = 'abastevo A app icon'
        if white:
            ET.SubElement(root,S('rect'),{'width':str(size),'height':str(size),'fill':'#ffffff'})
        group = ET.SubElement(root,S('g'),{'transform':f'translate({tx:.9f} {ty:.9f}) scale({scale:.12f})'})
        for child in master:
            if child.tag not in (S('title'),S('desc')):
                group.append(copy.deepcopy(child))
        return root
    save_xml(svg_asset(1024,icon_scale,ix,iy,True),BRAND/'abastevo-app-icon.svg')
    adaptive = tmp/'adaptive.svg'
    save_xml(svg_asset(108,adaptive_scale,ax,ay,True),adaptive)
    render(BRAND/'abastevo-app-icon.svg',tmp/'icon.png',1024)
    opaque = Image.open(tmp/'icon.png').convert('RGB')
    opaque.save(BRAND/'abastevo-app-icon-1024.png',optimize=True)
    # Keep native VectorDrawable paths/clips/gradient stops exactly unchanged.
    ET.register_namespace('android',ANDROID_NS)
    ET.register_namespace('aapt','http://schemas.android.com/aapt')
    native = ET.parse(BRAND/'abastevo_logo_android.xml').getroot()
    native.set(A('width'),'108dp');native.set(A('height'),'108dp')
    native.set(A('viewportWidth'),'108');native.set(A('viewportHeight'),'108')
    placement = ET.Element('group',{A('scaleX'):f'{adaptive_scale:.12f}',A('scaleY'):f'{adaptive_scale:.12f}',A('translateX'):f'{ax:.9f}',A('translateY'):f'{ay:.9f}'})
    for child in list(native):
        native.remove(child);placement.append(child)
    native.append(placement)
    save_xml(native,RES/'drawable/ic_launcher_foreground.xml')
    # One flat silhouette for opt-in OS themed icons; colors are OS-controlled.
    mono = ET.Element('vector',dict(native.attrib))
    mg = ET.SubElement(mono,'group',dict(placement.attrib))
    for clip in master.findall('.//'+S('clipPath')):
        path = clip.find(S('path'))
        ET.SubElement(mg,'path',{A('pathData'):path.attrib['d'],A('fillColor'):'#000000'})
    save_xml(mono,RES/'drawable/ic_launcher_monochrome.xml')
    old = RES/'drawable-nodpi/ic_launcher_foreground.png'
    if old.exists(): old.unlink()
    for density,size in [('mdpi',48),('hdpi',72),('xhdpi',96),('xxhdpi',144),('xxxhdpi',192)]:
        render(BRAND/'abastevo-app-icon.svg',tmp/'legacy.png',size)
        image = Image.open(tmp/'legacy.png').convert('RGB')
        folder=RES/f'mipmap-{density}';folder.mkdir(parents=True,exist_ok=True)
        image.save(folder/'ic_launcher.png',optimize=True)
        # Let the launcher apply its mask; do not bake shadows/rounding into artwork.
        image.save(folder/'ic_launcher_round.png',optimize=True)
    for version in (26,33):
        for name in ('ic_launcher','ic_launcher_round'):
            root=ET.Element('adaptive-icon')
            ET.SubElement(root,'background',{A('drawable'):'@color/ic_launcher_background'})
            ET.SubElement(root,'foreground',{A('drawable'):'@drawable/ic_launcher_foreground'})
            if version>=33:
                ET.SubElement(root,'monochrome',{A('drawable'):'@drawable/ic_launcher_monochrome'})
            save_xml(root,RES/f'mipmap-anydpi-v{version}/{name}.xml')
    # Native iPhone/iPad legacy slot coverage plus the App Store master.
    slots=[('iphone','20x20',[2,3]),('iphone','29x29',[2,3]),('iphone','40x40',[2,3]),('iphone','60x60',[2,3]),
           ('ipad','20x20',[1,2]),('ipad','29x29',[1,2]),('ipad','40x40',[1,2]),('ipad','76x76',[1,2]),('ipad','83.5x83.5',[2]),('ios-marketing','1024x1024',[1])]
    IOS.mkdir(parents=True,exist_ok=True);entries=[]
    for idiom,size,scales in slots:
        for scale in scales:
            px=round(float(size.split('x')[0])*scale);filename=f'icon-{px}.png'
            render(BRAND/'abastevo-app-icon.svg',tmp/'ios.png',px)
            Image.open(tmp/'ios.png').convert('RGB').save(IOS/filename,optimize=True)
            entries.append({'idiom':idiom,'size':size,'scale':f'{scale}x','filename':filename})
    (IOS/'Contents.json').write_text(json.dumps({'images':entries,'info':{'version':1,'author':'xcode'}},indent=2)+'\n')
    (IOS.parent/'Contents.json').write_text(json.dumps({'info':{'version':1,'author':'xcode'}},indent=2)+'\n')
    # Visual masks for inspection only; shipped assets remain unmasked.
    render(adaptive,tmp/'adaptive.png',1080)
    foreground=Image.open(tmp/'adaptive.png').convert('RGB')
    from PIL import ImageDraw
    preview=Image.new('RGB',(960,360),'#eef1f5');draw=ImageDraw.Draw(preview)
    visible=foreground.crop((180,180,900,900)).resize((256,256),Image.Resampling.LANCZOS)
    for index,shape in enumerate(('square','rounded','circle')):
        mask=Image.new('L',(256,256),0);md=ImageDraw.Draw(mask)
        if shape=='circle':md.ellipse((0,0,255,255),fill=255)
        elif shape=='rounded':md.rounded_rectangle((0,0,255,255),radius=55,fill=255)
        else:md.rectangle((0,0,255,255),fill=255)
        preview.paste(visible,(32+index*320,45),mask);draw.text((32+index*320,318),shape,fill='#13233e')
    preview.save(BRAND/'abastevo-app-icon-preview.png',optimize=True)
assert hashlib.sha256(master_path.read_bytes()).hexdigest()==master_hash
(BRAND/'app-icon-provenance.json').write_text(json.dumps({'task':'P00-T04','source':'abastevo-logo.svg','source_sha256':master_hash,'method':'Native SVG geometry/gradients preserved; Inkscape exports, white opaque background; no generative image tool','bounds':bbox,'android_safe_zone_radius_dp':32.5,'android_transform':{'scale':adaptive_scale,'x':ax,'y':ay},'ios_master_pixels':1024,'ios_mark_width_pixels':760,'ios_slots':len(entries),'ios_runtime':'NOT_COMPILED: no Xcode application target / Mac on this host'},indent=2)+'\n')
print('Android native adaptive/themed/fallback assets and iPhone/iPad AppIcon catalog exported.')
