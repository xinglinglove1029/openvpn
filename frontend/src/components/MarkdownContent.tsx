import { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface MarkdownContentProps {
  content: string;
}

/** Markdown 渲染组件，针对移动端做了响应式适配 */
const MarkdownContent = memo(function MarkdownContent({ content }: MarkdownContentProps) {
  if (!content) return <span className="text-muted-foreground">-</span>;

  return (
    <div className="markdown-body
      text-sm leading-relaxed
      [&_h1]:text-xl [&_h1]:font-bold [&_h1]:mt-4 [&_h1]:mb-3 [&_h1]:text-foreground
      [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:mt-4 [&_h2]:mb-2 [&_h2]:text-foreground
      [&_h3]:text-base [&_h3]:font-semibold [&_h3]:mt-3 [&_h3]:mb-2 [&_h3]:text-foreground
      [&_h4]:text-sm [&_h4]:font-semibold [&_h4]:mt-3 [&_h4]:mb-1 [&_h4]:text-foreground
      [&_p]:mb-2 [&_p]:text-foreground/85
      [&_ul]:mb-3 [&_ul]:pl-5 [&_ul]:space-y-1
      [&_ol]:mb-3 [&_ol]:pl-5 [&_ol]:space-y-1
      [&_li]:text-foreground/85 [&_li]:break-words
      [&_li_strong]:text-foreground [&_li_strong]:mr-1
      [&_strong]:font-semibold [&_strong]:text-foreground
      [&_em]:italic [&_em]:text-foreground/80
      [&_a]:text-blue-500 [&_a]:underline [&_a]:break-all
      [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-xs [&_code]:font-mono [&_code]:break-all
      [&_pre]:bg-muted [&_pre]:rounded-md [&_pre]:p-3 [&_pre]:mb-3 [&_pre]:overflow-x-auto [&_pre]:text-xs
      [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_pre_code]:rounded-none [&_pre_code]:text-xs
      [&_blockquote]:border-l-3 [&_blockquote]:border-muted-foreground/30 [&_blockquote]:pl-3 [&_blockquote]:mb-3 [&_blockquote]:text-muted-foreground [&_blockquote]:italic
      [&_hr]:border-muted-foreground/20 [&_hr]:my-4
      [&_table]:w-full [&_table]:mb-3 [&_table]:text-xs [&_table]:border-collapse
      [&_th]:border [&_th]:border-border [&_th]:px-2 [&_th]:py-1.5 [&_th]:bg-muted/50 [&_th]:text-left [&_th]:font-semibold
      [&_td]:border [&_td]:border-border [&_td]:px-2 [&_td]:py-1.5 [&_td]:break-words
      [&_img]:max-w-full [&_img]:rounded-md

      [&_del]:line-through [&_del]:text-muted-foreground
      [&_input[type=checkbox]]:mr-1.5

      /* 移动端适配 */
      max-sm:[&_h1]:text-lg
      max-sm:[&_h2]:text-base
      max-sm:[&_h3]:text-sm
      max-sm:[&_ul]:pl-4
      max-sm:[&_ol]:pl-4
      max-sm:[&_pre]:p-2
      max-sm:[&_pre]:text-[11px]
      max-sm:[&_code]:text-[11px]
      max-sm:[&_table]:text-[11px]
    ">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>
        {content}
      </ReactMarkdown>
    </div>
  );
});

export default MarkdownContent;
