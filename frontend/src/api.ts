export async function api<T>(path:string,payload:unknown):Promise<T>{
 const response=await fetch('/api/'+path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)})
 const result=await response.json()
 if(!response.ok)throw new Error(result.error || '请求失败')
 return result as T
}
export function save(name:string,data:BlobPart,type='application/json'){
 const url=URL.createObjectURL(new Blob([data],{type}));const link=document.createElement('a');link.href=url;link.download=name;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)
}
export const pretty=(value:unknown)=>JSON.stringify(value,null,2)
export const message=(error:unknown)=>error instanceof Error?error.message:String(error)
