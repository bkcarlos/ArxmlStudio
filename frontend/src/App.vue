<script setup lang="ts">
import {ref,computed,watch} from 'vue'
import type {Document,InputFile,Comparison} from './types'
import {api,save,pretty,message} from './api'
import ServiceDetail from './ServiceDetail.vue'
import ExportDialog from './ExportDialog.vue'
type Slot='A'|'B'
const files=ref<Partial<Record<Slot,InputFile>>>({});const docs=ref<Partial<Record<Slot,Document>>>({});const source=ref<Slot>('A');const selected=ref('');const search=ref('');const busy=ref(false);const status=ref('准备就绪 · 文件只发送到本机服务');const tab=ref('browse');const comparison=ref<Comparison>();const exporting=ref(false)
const doc=computed(()=>docs.value[source.value]);const services=computed(()=>doc.value?.services.filter(s=>[s.name,s.path,...s.methods.map(m=>m.name),...s.events.map(m=>m.name),...s.fields.map(m=>m.name)].some(n=>n.toLowerCase().includes(search.value.toLowerCase())))??[])
const service=computed(()=>services.value.find(s=>s.path===selected.value)??services.value[0]);const labels={services:'服务 / 接口',methods:'Methods',events:'Events',fields:'Fields',types:'数据类型'}
watch(source,()=>selected.value='')
async function guarded(fn:()=>Promise<void>){if(busy.value)return;busy.value=true;try{await fn()}catch(e){status.value='操作失败：'+message(e)}finally{busy.value=false}}
async function load(slot:Slot,file:File){
 if(file.size>20*1024*1024)throw Error('文件超过 20 MiB');status.value='正在解析 '+file.name+'…'
 const data=await new Promise<string>((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(',')[1]);reader.onerror=()=>reject(Error('读取文件失败'));reader.readAsDataURL(file)})
 const input={name:file.name,data};const parsed=await api<Document>('parse',input);files.value[slot]=input;docs.value[slot]=parsed;source.value=slot;selected.value='';comparison.value=undefined
 if(files.value.A&&files.value.B)comparison.value=await api<Comparison>('compare',{before:files.value.A,after:files.value.B})
 status.value='已解析 '+file.name+' · '+parsed.summary.services+' 个服务 / 通信接口'
}
function upload(slot:Slot,event:Event){const target=event.target as HTMLInputElement;const file=target.files?.[0];if(file)guarded(()=>load(slot,file));target.value=''}
function drop(slot:Slot,event:DragEvent){const file=event.dataTransfer?.files[0];if(file)guarded(()=>load(slot,file))}
function demo(){guarded(async()=>{for(const [slot,name] of [['A','vehicle-v1.arxml'],['B','vehicle-v2.arxml']] as const){const r=await fetch('/examples/'+name);if(!r.ok)throw Error('演示文件加载失败');await load(slot,new File([await r.blob()],name))}source.value='A';tab.value='browse';status.value='演示 A/B 已加载 · 当前浏览文件 A'})}
</script>
<template><header><div class="logo">A<span>↗</span></div><div><strong>ARXML Studio</strong><small>服务探索 · 版本对比 · 配置生成</small></div><span class="local">● 本地工作空间</span></header>
<main><section class="intro"><div><div class="eyebrow">AUTOSAR / SERVICE EXPLORER</div><h1>从接口定义，到通信配置。</h1><p>打开 ARXML，浏览服务结构，对比版本，导出 vsomeip 与 CommonAPI 文件。</p></div><button :disabled="busy" @click="demo">加载演示文件</button></section>
<section class="uploads"><label v-for="slot in (['A','B'] as const)" :key="slot" class="upload" @dragover.prevent @drop.prevent="drop(slot,$event)"><b>{{slot==='A'?'01 / 基准文件 A':'02 / 对比文件 B'}}</b><span>{{files[slot]?.name||'选择或拖入 .arxml 文件'}}</span><input type="file" :aria-label="`文件 ${slot}`" accept=".arxml,.xml" :disabled="busy" @change="upload(slot,$event)"></label></section>
<div id="status" role="status" aria-live="polite">{{status}}</div><section class="stats"><div v-for="(label,key) in labels" :key="key" class="stat"><b>{{doc?.summary[key]??'—'}}</b><span>{{label}}</span></div></section>
<nav><button :class="{active:tab==='browse'}" @click="tab='browse'">服务浏览</button><button :class="{active:tab==='diff'}" @click="tab='diff'">版本对比 {{comparison?`(${comparison.changes.length})`:''}}</button><div class="spacer"></div><label>当前文件 <select v-model="source" :disabled="busy"><option value="A">文件 A</option><option value="B">文件 B</option></select></label><button :disabled="!doc" @click="doc&&save(doc.source+'.json',pretty(doc))">导出解析 JSON</button></nav>
<section v-if="tab==='browse'" class="workspace"><aside><input v-model="search" type="search" placeholder="搜索服务、方法、事件…" aria-label="搜索服务"><div id="services"><button v-for="s in services" :key="s.path" :class="{selected:s.path===service?.path}" @click="selected=s.path"><b>{{s.name}}</b><small>{{s.methods.length}} 方法 · {{s.events.length}} 事件 · {{s.fields.length}} 字段</small><small>{{s.path}}</small></button><p v-if="!services.length" class="meta">暂无匹配服务</p></div></aside><article><ServiceDetail v-if="service" :key="source+service.path" :service="service" @export="exporting=true"/><div v-else class="empty">选择文件与服务，查看接口结构</div></article></section>
<section v-else><div class="toolbar"><p>对比方向：A → B · 忽略排版、命名空间前缀和 UUID 变化</p><button :disabled="!comparison" @click="comparison&&save('arxml-diff.json',pretty(comparison))">导出差异报告</button></div><div v-if="!comparison" class="empty">加载 A、B 两份文件以比较版本</div><template v-else><p>新增 {{comparison.summary.added}} · 删除 {{comparison.summary.removed}} · 修改 {{comparison.summary.modified}}</p><div v-if="!comparison.changes.length" class="empty">支持比较的接口与数据类型没有变化</div><div v-for="(change,index) in comparison.changes" :key="index" class="diff-card"><span class="badge" :class="change.status">{{({added:'新增',removed:'删除',modified:'修改'} as Record<string,string>)[change.status]}}</span><span class="path">{{change.path}}</span><div class="diff-values"><div><small>A / 修改前</small><pre>{{pretty(change.before)}}</pre></div><div><small>B / 修改后</small><pre>{{pretty(change.after)}}</pre></div></div></div></template></section>
<section id="warnings" v-if="doc?.warnings.length"><details><summary>{{doc.warnings.length}} 条解析提示</summary><p v-for="(warning,i) in doc.warnings" :key="i">{{warning.message}} {{warning.reference}} {{warning.path}}</p></details></section>
<footer>ARXML Studio / 0.2 <span>Go + Vue · 单文件运行</span></footer></main>
<ExportDialog v-if="exporting&&service&&files[source]" :service="service" :file="files[source]!" :source="source" @close="exporting=false"/>
</template>
