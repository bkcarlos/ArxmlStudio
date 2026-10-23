<script setup lang="ts">
import {ref,onMounted} from 'vue'
import type {InputFile,Service,Settings,Bundle} from './types'
import {api,save,pretty,message} from './api'
const props=defineProps<{file:InputFile;service:Service;source:string}>()
const emit=defineEmits<{close:[]}>()
const dialog=ref<HTMLDialogElement>();const settings=ref<Settings>();const error=ref('');const success=ref('');const busy=ref(false);const mapping=ref('{}')
onMounted(async()=>{dialog.value?.showModal();try{settings.value=await api<Settings>('settings',{file:props.file,path:props.service.path})}catch(e){error.value=message(e)}})
function setGroups(name:string,event:Event){if(settings.value)settings.value.members[name].eventgroups=(event.target as HTMLInputElement).value.split(',').map(s=>s.trim()).filter(Boolean)}
async function download(){if(!settings.value)return;busy.value=true;error.value='';success.value='';try{
 const parsed:unknown=JSON.parse(mapping.value);if(!parsed||typeof parsed!=='object'||Array.isArray(parsed)||Object.values(parsed).some(v=>typeof v!=='string'))throw Error('类型映射必须是路径到类型名称的 JSON 对象')
 settings.value.type_mapping=parsed as Record<string,string>
 const bundle=await api<Bundle>('export',{file:props.file,path:props.service.path,settings:settings.value})
 save(bundle.filename,Uint8Array.from(atob(bundle.data),c=>c.charCodeAt(0)),'application/zip');success.value='已生成：'+Object.keys(bundle.files).join('、')
 }catch(e){error.value=message(e)}finally{busy.value=false}}
</script>
<template><dialog ref="dialog" @close="emit('close')"><button class="close" aria-label="关闭" @click="dialog?.close()">×</button><div class="eyebrow">EXPORT CONFIGURATION</div><h2>导出通信配置</h2><p>文件 {{source}} · {{service.path}}</p>
 <form v-if="settings" @submit.prevent="download"><div class="form-grid">
 <label>导出角色<select v-model="settings.role" @change="success=''" :disabled="busy"><option value="server">服务提供方 · Server / Stub</option><option value="client">服务调用方 · Client / Proxy</option></select></label>
 <label>传输协议<select v-model="settings.transport"><option value="udp">UDP</option><option value="tcp">TCP</option></select></label>
 <label>CommonAPI 包名<input v-model="settings.package" required></label><label>CommonAPI 实例名<input v-model="settings.instance_name" required></label>
 <label>Application 名称<input v-model="settings.application" required></label><label>Application ID<input v-model="settings.application_id" placeholder="例如 0x1001" required></label>
 <label>本机 IPv4<input v-model="settings.unicast" placeholder="进程所在主机地址" required></label><label v-if="settings.role==='server'">服务端口<input v-model="settings.port" placeholder="例如 30509" required></label>
 <label>Service ID<input v-model="settings.service_id" required></label><label>Instance ID<input v-model="settings.instance_id" required></label><label>Major version<input v-model="settings.major" required></label><label>Minor version<input v-model="settings.minor" required></label>
 </div><p>双方需使用相同接口与通信 ID。客户端使用服务发现，无需配置服务端口。ID 支持十进制或 0x 十六进制。</p>
 <h3>成员通信 ID</h3><div v-for="member in [...service.methods,...service.events,...service.fields]" :key="member.path" class="member"><b>{{member.name}}</b><div class="form-grid">
 <template v-if="member.access"><label v-for="access in (['getter','setter','notifier'] as const).filter(k=>member.access?.['HAS-'+k.toUpperCase()]==='true')" :key="access">{{access}} ID<input v-model="settings.members[member.name][`${access}_id`]" required></label></template>
 <label v-else>Method / Event ID<input v-model="settings.members[member.name].id" required></label>
 <label v-if="service.events.some(e=>e.path===member.path)||member.access?.['HAS-NOTIFIER']==='true'">Eventgroup IDs<input :value="settings.members[member.name].eventgroups?.join(', ')" @input="setGroups(member.name,$event)" placeholder="逗号分隔，例如 1, 2" required></label>
 </div></div>
 <details><summary>高级：类型映射与参数预览</summary><p>仅在确认等价时，将完整 ARXML 类型路径映射为 Franca 基础类型。</p><label>类型映射 JSON<textarea v-model="mapping" spellcheck="false" style="height:120px"></textarea></label><pre>{{pretty(settings)}}</pre></details>
 <p class="error" role="alert">{{error}}</p><p class="success" role="status">{{success}}</p><button class="primary" :disabled="busy">{{busy?'正在生成…':'生成并下载 ZIP'}}</button></form><p v-else role="status">{{error||'正在读取部署参数…'}}</p>
 </dialog></template>
