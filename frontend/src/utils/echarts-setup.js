import { use } from 'echarts/core'
import { CanvasRenderer, SVGRenderer } from 'echarts/renderers'
import {
  BarChart, LineChart, RadarChart, HeatmapChart, ScatterChart
} from 'echarts/charts'
import {
  TitleComponent, TooltipComponent, LegendComponent, GridComponent,
  DataZoomComponent, ToolboxComponent, VisualMapComponent
} from 'echarts/components'

use([
  CanvasRenderer, SVGRenderer,
  BarChart, LineChart, RadarChart, HeatmapChart, ScatterChart,
  TitleComponent, TooltipComponent, LegendComponent, GridComponent,
  DataZoomComponent, ToolboxComponent, VisualMapComponent
])
