# coding=utf-8
import tkinter as tk
import re
import sys

title = sys.argv[1] if len(sys.argv) > 1 else ""
msg_file = sys.argv[2] if len(sys.argv) > 2 else ""

with open(msg_file, encoding="utf-8") as f:
    raw = f.read()

root = tk.Tk()
root.title(title)
root.attributes("-topmost", True)
root.configure(bg="#1e1e1e")

screen_w = root.winfo_screenwidth()
screen_h = root.winfo_screenheight()
win_w, win_h = 420, 200
x = screen_w - win_w - 30
y = screen_h - win_h - 60
root.geometry(f"{win_w}x{win_h}+{x}+{y}")

tk.Frame(root, bg="#22d3ee", height=4).pack(fill="x")

content = tk.Frame(root, bg="#1e1e1e")
content.pack(fill="both", expand=True, padx=20, pady=16)

text = tk.Text(content, bg="#1e1e1e", fg="#e2e8f0",
               font=("Microsoft YaHei", 13), wrap="word",
               relief="flat", highlightthickness=0,
               width=42, height=4, cursor="arrow")
text.pack(fill="x")
text.tag_configure("bold", font=("Microsoft YaHei", 13, "bold"))
text.tag_configure("dim", foreground="#94a3b8")
text.tag_configure("mono", font=("Consolas", 12))

pos = 0
for bold_m in re.finditer(r'\*\*(.+?)\*\*', raw):
    text.insert("end", raw[pos:bold_m.start()], ())
    text.insert("end", bold_m.group(1), ("bold",))
    pos = bold_m.end()
text.insert("end", raw[pos:], ())
text.config(state="disabled")

btn_frame = tk.Frame(content, bg="#1e1e1e")
btn_frame.pack(fill="x", pady=(12, 0))

def copy_all():
    root.clipboard_clear()
    root.clipboard_append(raw)
    copy_btn.config(text="\u2713 \u5df2\u590d\u5236", bg="#22c55e")
    root.after(1500, root.destroy)

copy_btn = tk.Button(btn_frame, text="\u590d\u5236\u5185\u5bb9", command=copy_all,
                    bg="#334155", fg="#e2e8f0", activebackground="#475569",
                    activeforeground="#fff", relief="flat", cursor="hand2",
                    font=("Microsoft YaHei", 12), padx=16, pady=8)
copy_btn.pack(side="left", fill="x", expand=True, padx=(0, 8))

def dismiss():
    root.destroy()

ok_btn = tk.Button(btn_frame, text="\u5173\u95ed", command=dismiss,
                  bg="#22d3ee", fg="#000", activebackground="#06b6d4",
                  activeforeground="#000", relief="flat", cursor="hand2",
                  font=("Microsoft YaHei", 12, "bold"), padx=16, pady=8)
ok_btn.pack(side="left", fill="x", expand=True)

root.mainloop()
