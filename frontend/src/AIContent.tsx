import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Children, isValidElement, type ReactNode } from "react";
import type { LocateSource } from "./source-location";

function plainText(children: ReactNode): string {
  return Children.toArray(children).map(child => typeof child === "string" || typeof child === "number" ? String(child) : isValidElement<{children?:ReactNode}>(child) ? plainText(child.props.children) : "").join("");
}

/** Shared reading view for both streamed results and saved AI responses. */
export default function AIContent({ text,onLocate,sourceText="" }: { text: string;onLocate?:LocateSource;sourceText?:string }) {
  const quoted = (children:ReactNode) => Children.map(children,child=>{
    if(typeof child!=="string"||!onLocate)return child;
    return child.split(/([“「『][^”」』\n]+[”」』])/u).map((part,i)=>{
      const quote=/^[“「『]([\s\S]+)[”」』]$/u.exec(part)?.[1];
      return quote&&sourceText.includes(quote)?<button key={i} className="source-quote" title="定位原文" onClick={()=>onLocate(quote)}>{part}</button>:part;
    });
  });
  return <div className="ai-content"><Markdown
    remarkPlugins={[remarkGfm]}
    skipHtml
    components={{
      p: ({children}) => { const content=plainText(children).trim(),quote=/^(?:原文|原句|引用原文)[：:]\s*([\s\S]+)$/.exec(content)?.[1]||content;return <p>{quoted(children)}{onLocate&&quote.length>1&&sourceText.includes(quote)&&<button className="source-locate" onClick={()=>onLocate(quote)}>定位原文</button>}</p>; },
      strong: ({children}) => <strong>{quoted(children)}</strong>,
      em: ({children}) => <em>{quoted(children)}</em>,
      li: ({children}) => <li>{quoted(children)}</li>,
      blockquote: ({children}) => { const quote=Children.toArray(children).map(child=>plainText(child).trim()).filter(Boolean).join("\n");return <blockquote>{children}{onLocate&&quote&&<button className="source-locate" onClick={()=>onLocate(quote)}>定位引用原文</button>}</blockquote>; },
      code: ({children,className}) => {const quote=plainText(children);return <code className={className}>{onLocate&&quote.trim()&&sourceText.includes(quote)?<button className="source-quote" title="定位原文" onClick={()=>onLocate(quote)}>{children}</button>:children}</code>;},
      a: ({ children, href }) => href
        ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
        : <span>{children}</span>,
      // Model output must not load remote images or tracking URLs automatically.
      img: ({ alt }) => <span className="muted">{alt || "图片"}</span>,
      table: ({ children }) => <div className="ai-table-scroll"><table>{children}</table></div>,
    }}
  >{text}</Markdown></div>;
}
