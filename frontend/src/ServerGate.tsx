import { useEffect,useState,type ReactNode } from "react";
import { api } from "./types";
export default function ServerGate({children}:{children:ReactNode}){
 const [ready,setReady]=useState(false),[login,setLogin]=useState(false),[token,setToken]=useState(""),[error,setError]=useState(""),[busy,setBusy]=useState(false);
 const check=async()=>{try{setError("");const info=await api<{remote:boolean;authenticated:boolean}>("/server-info");setLogin(info.remote&&!info.authenticated);setReady(!info.remote||info.authenticated)}catch(e){setError((e as Error).message)}};
 useEffect(()=>{void check()},[]);
 if(ready)return <>{children}</>;
 return <main className="server-login"><form className="card" onSubmit={async e=>{e.preventDefault();setBusy(true);try{await api("/auth/login","POST",{token});setToken("");await check()}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}>
 <div className="eyebrow">YANTAI SERVER</div><h1>连接你的写作空间</h1>
 {login?<><p className="muted">填写部署时配置的服务端访问密钥。</p><label>访问密钥<input type="password" autoComplete="off" value={token} onChange={e=>setToken(e.target.value)} required/></label><button className="btn" disabled={busy}>{busy?"登录中……":"登录服务端"}</button></>:<p>正在连接工作台……</p>}
 {error&&<><p className="info-note" role="alert">{error}</p><button type="button" className="btn secondary" onClick={()=>void check()}>重新连接</button></>}
 </form></main>;
}
