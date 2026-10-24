#!/usr/bin/env python3
"""Normalize original generated map pictograms and trace reproducible SVG masters.
Build-time tools only: Pillow, NumPy, VTracer and Inkscape. No app integration.
The source image is retained unchanged. Match the existing fuel asset pipeline.
"""
import argparse, hashlib, json, subprocess, tempfile, re
from pathlib import Path
import xml.etree.ElementTree as ET
import numpy as np
from PIL import Image, ImageDraw
import vtracer
NS='http://www.w3.org/2000/svg'
def run(source, name, ink, output):
    arr=np.asarray(Image.open(source).convert('RGBA'))
    opaque=arr[:,:,3]>=128
    y,x=np.where(opaque)
    if not len(x): raise ValueError('Empty artwork')
    arr=arr[y.min():y.max()+1,x.min():x.max()+1]
    visible=arr[:,:,3]>=128
    rgb=arr[:,:,:3].astype(np.int16)
    colored=visible & ((rgb.max(2)-rgb.min(2))>35)
    white=visible & ~colored
    side=round(max(arr.shape[:2])/0.875)
    top,left=(side-arr.shape[0])//2,(side-arr.shape[1])//2
    rgba=np.zeros((side,side,4),dtype=np.uint8)
    block=rgba[top:top+arr.shape[0],left:left+arr.shape[1]]
    ink_rgb=tuple(bytes.fromhex(ink[1:]))
    block[colored,:3]=ink_rgb; block[white,:3]=255; block[visible,3]=255
    normalized=output/'transparent'/f'{name}.png'
    normalized.parent.mkdir(parents=True,exist_ok=True)
    Image.fromarray(rgba).save(normalized,optimize=True)
    svg=ET.Element('svg',xmlns=NS,width='128',height='128',viewBox=f'0 0 {side*2} {side*2}')
    with tempfile.TemporaryDirectory(prefix='abastevo-map-trace-') as tmp:
      for idx,(mask,fill) in enumerate(((colored,ink),(white,'#FFFFFF'))):
        bitmap=np.full((side,side),255,dtype=np.uint8)
        bitmap[top:top+mask.shape[0],left:left+mask.shape[1]][mask]=0
        src,traced=Path(tmp)/f'mask-{idx}.png',Path(tmp)/f'trace-{idx}.svg'
        Image.fromarray(bitmap).resize((side*2,side*2),Image.Resampling.NEAREST).save(src)
        vtracer.convert_image_to_svg_py(str(src),str(traced),colormode='binary',mode='spline',filter_speckle=2,corner_threshold=60,length_threshold=2,max_iterations=10,splice_threshold=20,path_precision=8)
        for path in ET.parse(traced).getroot().findall(f'{{{NS}}}path'):
          if path.get('fill','').upper()!='#000000': raise ValueError('Unexpected trace ink')
          attrs={'d':path.attrib['d'],'fill':fill}
          if path.get('transform'): attrs['transform']=path.get('transform')
          ET.SubElement(svg,'path',attrs)
    vector=output/f'{name}.svg'
    ET.indent(svg);ET.ElementTree(svg).write(vector,encoding='utf-8',xml_declaration=True)
    target=output/'android'/f'{name}.png';target.parent.mkdir(exist_ok=True)
    def export(dst, size):
      subprocess.run(['inkscape',str(vector),f'--export-width={size}',f'--export-height={size}',f'--export-filename={dst}'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    export(target,128)
    with tempfile.TemporaryDirectory(prefix='abastevo-map-qa-') as tmp:
      rendered=Path(tmp)/'render.png';export(rendered,side)
      b=np.asarray(Image.open(rendered).convert('RGBA')); a=rgba[:,:,3]>=128; bvis=b[:,:,3]>=128
      dif_ink=((b[:,:,:3].astype(float)-np.array(ink_rgb))**2).sum(2)
      dif_white=((b[:,:,:3].astype(float)-255)**2).sum(2)
      refink=a & np.any(rgba[:,:,:3]!=255,axis=2); actualink=bvis & (dif_ink<dif_white)
      scores={}
      for key,ref,actual in [('silhouette',a,bvis),('ink',refink,actualink),('white',a & ~refink,bvis & ~actualink)]:
        union=(ref|actual).sum();score=float((ref&actual).sum()/union) if union else 1.
        scores[key+'_iou']=round(score,6)
        if score<.99: raise ValueError(f'{name}: {key} below 99%: {score}')
    metadata={'source_type':'original built-in ImageGen output','raw_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'normalization':'alpha >=128; saturated ink + white; transparent padding; trace at 2x nearest-neighbor; no source downsample','category_ink':ink,'category_semantics':'station service type, not fuel grade','source_size':list(Image.open(source).size),'normalized_size':[side,side],'android_export_size':[128,128],'svg_paths':len(svg),'svg_sha256':hashlib.sha256(vector.read_bytes()).hexdigest(),'android_sha256':hashlib.sha256(target.read_bytes()).hexdigest(),'validation':scores,'integration_status':'PREPARED_ASSET / NOT_INTEGRATED','attribution':'Original generic Abastevo artwork, no upstream CoMaps or operator logos; repository MIT notice applies to the delivered source assets.'}
    (output/f'{name}.provenance.json').write_text(json.dumps(metadata,indent=2)+'\n')
    print(name,json.dumps(scores),f'{len(svg)} paths')
    return target
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--source',type=Path,required=True);p.add_argument('--name',required=True);p.add_argument('--color',required=True);p.add_argument('--output',type=Path,required=True);args=p.parse_args()
 if not re.fullmatch(r'ic_map_station_(combustion|electric)',args.name):p.error('Unknown map station type')
 args.output.mkdir(parents=True,exist_ok=True);run(args.source,args.name,args.color.upper(),args.output)
