// Uses an existing external Playwright/browser installation; no runtime dependency.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const dir = __dirname;
const root = path.resolve(dir, '../../../..');
const mime = { '.html':'text/html; charset=utf-8', '.css':'text/css', '.js':'text/javascript', '.svg':'image/svg+xml', '.png':'image/png', '.woff2':'font/woff2' };
const server = http.createServer((req,res) => {
  try {
    const url = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
    const source = url.startsWith('/docs/assets/landing/redesign-20261011/') || url.startsWith('/docs/assets/brand/') || url.startsWith('/landing/static/assets/');
    const base = source ? root : path.join(root, 'landing/dist');
    const file = path.resolve(base, '.' + (url === '/' ? '/index.html' : url));
    const allowedSourceRoots = [dir, path.join(root, 'docs/assets/brand'), path.join(root, 'landing/static/assets')];
    if (!file.startsWith(base + path.sep) || (source && !allowedSourceRoots.some(allowed => file.startsWith(allowed + path.sep)))) throw new Error('Invalid path');
    const content = fs.readFileSync(file);
    res.writeHead(200, {'Content-Type':mime[path.extname(file)] || 'application/octet-stream'});res.end(content);
  } catch { res.writeHead(404);res.end('Not found'); }
});
(async () => {
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const origin='http://127.0.0.1:'+server.address().port;
  const browser=await chromium.launch({headless:true,...(process.env.CHROMIUM_EXECUTABLE ? {executablePath:process.env.CHROMIUM_EXECUTABLE}:{}),args:['--no-sandbox']});
  try {
    const page=await browser.newPage({viewport:{width:1440,height:1100},deviceScaleFactor:2});
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await page.goto(origin+'/docs/assets/landing/redesign-20261011/landing-reference-v2.html');
    await page.evaluate(()=>document.fonts.ready);
    await page.locator('img').evaluateAll(images=>Promise.all(images.map(image=>image.decode())));
    const fonts=await page.evaluate(()=>({manrope:document.fonts.check('700 16px Manrope'),source:document.fonts.check('400 16px "Source Sans 3"'),heroPhones:document.querySelector('.hero').querySelectorAll('img').length}));
    await page.screenshot({path:path.join(dir,'landing-reference-v2-desktop.png'),fullPage:true});
    await page.locator('#next').click();
    if(await page.locator('#lesson-image').getAttribute('src')!=='screenshots/city-picker.png')throw new Error('Next did not select actual city capture');
    await page.locator('#previous').click();
    if(await page.locator('#progress').innerText()!=='Etapa 1 de 2')throw new Error('Previous did not restore first step');
    const overflow=[];
    for(const width of [320,375,768,1024,1440]) { await page.setViewportSize({width,height:1100});overflow.push({width,overflow:await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)}); }
    await page.setViewportSize({width:375,height:900});
    await page.screenshot({path:path.join(dir,'landing-reference-v2-mobile.png'),fullPage:true});
    const production=await browser.newPage({viewport:{width:1440,height:1000}});
    const fontResponses=[];
    production.on('response',response=>{if(response.url().endsWith('.woff2'))fontResponses.push({path:new URL(response.url()).pathname,status:response.status()});});
    await production.goto(origin+'/');
    await production.evaluate(()=>document.fonts.ready);
    const computed=await production.evaluate(()=>({body:getComputedStyle(document.body).fontFamily,h1:getComputedStyle(document.querySelector('h1')).fontFamily,loadedFonts:[...document.fonts].filter(font=>font.status==='loaded').map(font=>font.family)}));
    if(!computed.loadedFonts.includes('Manrope') || !computed.loadedFonts.includes('Source Sans 3') || fontResponses.some(font=>font.status!==200))throw new Error('Emitted font load failed');
    if(overflow.some(item=>item.overflow) || errors.length || fonts.heroPhones!==0)throw new Error('Reference layout check failed');
    const result={fonts,computed,fontResponses,productionFontCheck:'Actual HTTP landing/dist output, no CSS injection',overflow,errors,viewport:{desktop:1440,mobile:375,deviceScaleFactor:2},prototype:'HTML/CSS composition with original PNG screens; no generated phone UI'};
    fs.writeFileSync(path.join(dir,'browser-verification.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));
  } finally { await browser.close(); }
})().catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>server.close());
