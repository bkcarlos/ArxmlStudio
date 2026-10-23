export interface Definition {tag:string;text:string;attributes:Record<string,string>;children:Definition[]}
export interface Member {name:string;path:string;type_ref:string;direction:string;definition:Definition;arguments?:Member[];access?:Record<string,string>}
export interface Deployment {name:string;path:string;service_id:string;major_version:string;minor_version:string;definition:Definition}
export interface Service {name:string;path:string;kind:string;methods:Member[];events:Member[];fields:Member[];data_elements:Member[];deployments:Deployment[]}
export interface Document {source:string;services:Service[];types:unknown[];summary:Record<string,number>;warnings:{path?:string;reference?:string;message:string}[]}
export interface InputFile {name:string;data:string}
export interface Comparison {before:string;after:string;summary:Record<string,number>;changes:{status:string;path:string;before:unknown;after:unknown}[]}
export type Numeric = string | number | null
export interface MemberSettings {id?:Numeric;getter_id?:Numeric;setter_id?:Numeric;notifier_id?:Numeric;eventgroups?:Numeric[]}
export interface Settings {package:string;role:'server'|'client';application:string;application_id:Numeric;unicast:string;transport:'tcp'|'udp';port:Numeric;service_id:Numeric;instance_id:Numeric;instance_name:string;major:Numeric;minor:Numeric;members:Record<string,MemberSettings>;type_mapping:Record<string,string>}
export interface Bundle {filename:string;data:string;files:Record<string,string>}
