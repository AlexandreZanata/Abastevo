#!/usr/bin/env python3
"""Remove the one unwanted generated interior void from the electric SVG.
This explicit user-requested vector edit follows tracing; raw generated art stays
unchanged. Exact path signature guards prevent filling intentional apertures.
"""
from pathlib import Path
import argparse, hashlib, json, re, subprocess, tempfile
import xml.etree.ElementTree as ET
import numpy as np
from PIL import Image

def export(src,dst,size):
 subprocess.run(['inkscape',str(src),f'--export-width={size}',f'--export-height={size}',f'--export-filename={dst}'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
def repair(root):
 name='ic_map_station_electric'; path=root/f'{name}.svg'; tree=ET.parse(path); svg=tree.getroot()
 first=svg[0]; parts=re.findall(r'M[^M]*',first.attrib['d'])
 if len(parts)==5:
  print('Electric SVG already repaired'); return
 if len(parts)!=6 or not parts[-1].startswith('M126 944 '):
  raise ValueError('Unexpected source trace: refuse to alter another aperture')
 nums=list(map(float,re.findall(r'-?\d*\.?\d+',parts[-1])));xs=nums[::2];ys=nums[1::2]
 bounds=(min(xs),min(ys),max(xs),max(ys))
 if bounds!=(10.,944.,240.,1150.):raise ValueError('Unexpected repair bounds')
 before_hash=hashlib.sha256(path.read_bytes()).hexdigest()
 meta_path=root/f'{name}.provenance.json'; meta=json.loads(meta_path.read_text());side=meta['normalized_size'][0]
 with tempfile.TemporaryDirectory(prefix='abastevo-electric-repair-') as tmp:
  before=Path(tmp)/'before.png';after=Path(tmp)/'after.png'
  export(path,before,side)
  first.set('d',' '.join(parts[:-1]));ET.indent(svg);tree.write(path,encoding='utf-8',xml_declaration=True)
  export(path,after,side)
  a=np.asarray(Image.open(before).convert('RGBA'));b=np.asarray(Image.open(after).convert('RGBA'))
  changed=np.any(a!=b,axis=2);yy,xx=np.where(changed)
  if not len(xx):raise ValueError('No repair rendered')
  # Local void + the existing first path translation, converted from 2x trace.
  allowed=np.zeros(changed.shape,dtype=bool);allowed[546:653,294:413]=True
  if np.any(changed & ~allowed):raise ValueError('Unexpected changes outside the one void')
  if b[599,354,3]!=255:raise ValueError('Repaired interior is not opaque')
  if tuple(b[599,354,:3])!=(0,121,107):raise ValueError('Interior does not match category ink')
  # Keep the pre-repair generated normalization for honest provenance.
  old=root/'transparent'/f'{name}.png';rawnorm=root/'sources'/f'{name}.normalized-before-svg-repair.png'
  if not rawnorm.exists():rawnorm.write_bytes(old.read_bytes())
  old.write_bytes(after.read_bytes())
  export(path,root/'android'/f'{name}.png',128)
  box=[int(xx.min()),int(yy.min()),int(xx.max()+1),int(yy.max()+1)]
 meta['pre_correction_trace_validation']=meta.pop('validation',{})
 meta['manual_svg_correction']={'request':'Fill the unwanted interior hole; preserve display, lightning bolt and plug.','method':'Remove exactly the final enclosed subpath of the first teal filled path.','removed_subpath_local_control_bounds':list(bounds),'pre_correction_svg_sha256':before_hash,'changed_normalized_pixel_bounds':box,'outside_repair_region_unchanged':True,'interior_alpha':255,'interior_rgb':[0,121,107],'corrected_master':'transparent/ic_map_station_electric.png is rendered from the user-corrected SVG; raw source and pre-correction normalization retained.','reproduction':'Run trace-map-icons.py, then fix-map-electric-svg.py. Original raw-mask IoU belongs to pre-correction trace only, not to the intentional geometry change.'}
 meta['svg_sha256']=hashlib.sha256(path.read_bytes()).hexdigest();meta['android_sha256']=hashlib.sha256((root/'android'/f'{name}.png').read_bytes()).hexdigest()
 meta['normalization']+='; explicit final SVG void correction per user request'
 meta_path.write_text(json.dumps(meta,indent=2)+'\n')
 print('Repaired electric SVG; only changed pixel bounds:',box)
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--assets',type=Path,required=True);args=p.parse_args();repair(args.assets)
