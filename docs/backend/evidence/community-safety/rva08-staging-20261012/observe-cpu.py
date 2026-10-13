import json,time,subprocess
from pathlib import Path
p=json.loads(subprocess.check_output(['kubectl','-n','abastevo-temp','get','pod','abastevo-temp-db-0','-o','json']))
c=p['status']['containerStatuses'][0]
assert c['name']=='postgres'
cid=c['containerID'].split('://',1)[1]
r=json.loads(subprocess.check_output(['k3s','crictl','inspect',cid]))
pid=r['info']['pid']
group=Path(f'/proc/{pid}/cgroup').read_text().strip().split(':',2)[2]
base=Path('/sys/fs/cgroup')/group.lstrip('/')
end=time.monotonic()+150
while time.monotonic()<end:
 cpu=[int(n) for n in Path('/proc/stat').read_text().splitlines()[0].split()[1:9]]
 stat={k:int(v) for k,v in (l.split() for l in (base/'cpu.stat').read_text().splitlines())}
 pressure=(base/'cpu.pressure').read_text()
 totals={l.split()[0]:int(l.split()[-1].split('=')[1]) for l in pressure.splitlines()}
 print(json.dumps({'wall':time.time(),'monotonic':time.monotonic(),'host_cpu_ticks':cpu,'postgres_cpu':stat,'postgres_pressure_usec':totals}),flush=True)
 time.sleep(.1)
