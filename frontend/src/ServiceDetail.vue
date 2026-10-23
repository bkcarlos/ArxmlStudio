<script setup lang="ts">
import type {Service} from './types'
import {pretty} from './api'
defineProps<{service:Service}>()
defineEmits<{export:[]}>()
const sections=[{key:'methods',label:'方法 Methods'},{key:'events',label:'事件 Events'},{key:'fields',label:'字段 Fields'},{key:'data_elements',label:'数据元素 Data elements'}] as const
</script>
<template>
 <div class="toolbar"><div><span class="badge">{{service.kind}}</span><h2>{{service.name}}</h2></div><button class="primary" @click="$emit('export')">导出配置 ↗</button></div>
 <p class="path">{{service.path}}</p>
 <section v-for="section in sections" :key="section.key"><h3>{{section.label}} <span class="meta">{{service[section.key].length}}</span></h3>
 <details v-for="member in service[section.key]" :key="member.path" class="member"><summary><b>{{member.name}}</b><span class="meta">{{member.type_ref || `${member.arguments?.length ?? 0} 个参数`}}</span></summary>
 <table v-if="section.key==='methods'"><thead><tr><th>参数</th><th>方向</th><th>类型</th></tr></thead><tbody><tr v-for="arg in member.arguments" :key="arg.path"><td>{{arg.name}}</td><td>{{arg.direction}}</td><td>{{arg.type_ref}}</td></tr></tbody></table>
 <p v-if="member.access" class="meta"><span v-for="(value,key) in member.access" :key="key">{{key}}: {{value}}　</span></p><pre>{{pretty(member.definition)}}</pre></details>
 <p v-if="!service[section.key].length" class="meta">未定义</p></section>
 <h3>SOME/IP 部署</h3><details v-for="dep in service.deployments" :key="dep.path" class="member"><summary>{{dep.name}} · ID {{dep.service_id}} · v{{dep.major_version}}.{{dep.minor_version}}</summary><pre>{{pretty(dep.definition)}}</pre></details><p v-if="!service.deployments.length" class="meta">未找到关联部署；导出时需补充通信参数。</p>
</template>
