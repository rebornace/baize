function f(t){if(t.trim()==="")return{ok:!0,value:{}};const e={};for(const o of t.split(`
`)){const n=o.trim();if(n==="")continue;const i=n.indexOf("=");if(i<=0)return{ok:!1,message:`无效键值行：${n}`};const s=n.slice(0,i).trim(),u=n.slice(i+1).trim();if(!s)return{ok:!1,message:`无效键值行：${n}`};e[s]=u}return{ok:!0,value:e}}function a(t){return t?Object.entries(t).map(([r,e])=>`${r}=${e}`).join(`
`):""}function c(t){const r=t.trim();return r===""?[]:r.includes(`
`)?r.split(`
`).map(e=>e.trim()).filter(e=>e!==""):r.split(/\s+/).filter(e=>e!=="")}export{c as a,a as f,f as p};
