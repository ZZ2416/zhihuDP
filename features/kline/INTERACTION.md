# K线面板交互设计说明（INTERACTION）

> 版本：v0.1
> 日期：2026-08-16
> 分支：`feature/kline`
> 关联：`features/kline/SRS.md`、`features/kline/SDD.md`
> 目标：K线面板可交互——悬停/点击某根蜡烛，查看当日开/收/高/低/成交量

---

## 1. 需求（FR）

| # | 需求 | 验收 |
|---|---|---|
| FR-I1 | 鼠标悬停蜡烛时显示**十字光标**（竖线=日期、横线=价格）与**当日明细浮层** | 悬停任意蜡烛，浮层显示该日 开/收/高/低/成交量/涨跌幅 |
| FR-I2 | 被命中的蜡烛**高亮**（叠加半透明描边） | 命中的蜡烛与相邻蜡烛有明显区分 |
| FR-I3 | 浮层跟随指针移动，**不超出图表边界**（自动翻转） | 指针在图表边缘时浮层不溢出容器 |
| FR-I4 | 移动端支持：触摸锁定浮层，再次点击关闭 | touch 下可查看明细 |
| FR-I5 | 明暗主题自适应 | 浮层/十字线颜色随主题变化（复用 CSS 变量） |

## 2. 交互规格

### 2.1 命中检测（O(1)）

蜡烛等宽排列（`cw = plotW / n`），指针 x 直接换算下标：

```
idx = clamp( floor( (px - padL) / cw ), 0, n-1 )     // px 为 viewBox 坐标
```

### 2.2 坐标换算（唯一难点）

SVG 用 `viewBox="0 0 860 340"` + CSS `width:100%` 自适应，指针拿到 CSS 像素，需换算回 viewBox：

```js
const rect = svg.getBoundingClientRect();
const px = (e.clientX - rect.left) * (860 / rect.width);   // x 换算
const py = (e.clientY - rect.top)  * (340 / rect.height);  // y 换算
```

容器 `#kline-chart` 设 `position:relative` + SVG 设 `touch-action:none`（防触摸滑动干扰）。

### 2.3 浮层内容与位置

- 内容：日期、今开/今收/最高/最低、成交量（≥1万手显示「万手」）、相对昨收涨跌幅（红涨绿跌）
- 位置：跟随指针，`left = px + 12`，超出右边界则翻转到指针左侧；垂直方向优先在指针上方，空间不足则下方

### 2.4 十字光标与高亮

- 竖线：`x = x(idx)`（蜡烛中心），横线：`y = py`
- 虚线样式（`stroke-dasharray: 3 3`），颜色 `var(--faint)` 随主题
- 高亮：叠加一个半透明矩形（`fill: var(--accent-weak)`，opacity 0.5）覆盖命中蜡烛

### 2.5 触摸交互

- `touchstart` → 显示并**锁定**浮层（手指移开仍显示）
- `click`（或再次触摸）→ 切换锁定状态
- `mouseleave` → 隐藏（仅鼠标场景）

## 3. 实现范围

| 文件 | 改动 |
|---|---|
| `web/js/kline.js` | 渲染后附加十字线/高亮/浮层元素 + 事件监听（mousemove/mouseleave/touchstart/click） |
| `web/css/style.css` | `#kline-chart{position:relative}` + `.kline-tip` 浮层样式 + 十字线样式 |
| `web/js/app.js` | `renderKline(candles, quote)` 传入报价（用于首根涨跌幅计算） |

**后端零改动**：数据已在 `/api/kline` 响应中，纯前端增量。

## 4. 验收

| # | 项 | 通过标准 |
|---|---|---|
| 1 | 悬停明细 | 悬停任一根蜡烛，浮层显示该日完整明细，数值与数据一致 |
| 2 | 边界 | 悬停最左/最右/最上/最下蜡烛，浮层不溢出、可读 |
| 3 | 主题 | 明暗主题下浮层/十字线均清晰 |
| 4 | 移动端 | touch 查看并关闭浮层正常 |
| 5 | 回归 | 原有渲染/行情/SSE 功能不受影响 |
