import * as React from "react"
import { Settings, Save } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
    Tabs,
    TabsContent,
    TabsList,
    TabsTrigger,
} from "@/components/ui/tabs"
import { Switch } from "@/components/ui/switch"

export type AIProvider = "openai" | "claude" | "gemini" | "deepseek"

export interface ProviderConfig {
    enabled: boolean
    apiKey: string
    baseUrl: string
    model: string
}

export type AIConfig = Record<AIProvider, ProviderConfig>

const DEFAULT_CONFIG: AIConfig = {
    openai: {
        enabled: true,
        apiKey: "",
        baseUrl: "https://api.openai.com/v1",
        model: "gpt-4o",
    },
    claude: {
        enabled: false,
        apiKey: "",
        baseUrl: "https://api.anthropic.com",
        model: "claude-3-5-sonnet-20240620",
    },
    gemini: {
        enabled: false,
        apiKey: "",
        baseUrl: "https://generativelanguage.googleapis.com",
        model: "gemini-1.5-pro",
    },
    deepseek: {
        enabled: false,
        apiKey: "",
        baseUrl: "https://api.deepseek.com",
        model: "deepseek-chat",
    },
}

export function SettingsDialog() {
    const [open, setOpen] = React.useState(false)
    const [config, setConfig] = React.useState<AIConfig>(DEFAULT_CONFIG)

    // Load from localStorage on mount
    React.useEffect(() => {
        const saved = localStorage.getItem("rag_ai_config")
        if (saved) {
            try {
                const parsed = JSON.parse(saved)
                // Merge with default to ensure structure
                setConfig({ ...DEFAULT_CONFIG, ...parsed })
            } catch (e) {
                console.error("Failed to parse saved config", e)
            }
        }
    }, [])

    const handleSave = () => {
        localStorage.setItem("rag_ai_config", JSON.stringify(config))
        setOpen(false)
    }

    const updateProvider = (provider: AIProvider, updates: Partial<ProviderConfig>) => {
        setConfig(prev => ({
            ...prev,
            [provider]: { ...prev[provider], ...updates }
        }))
    }

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                <Button variant="ghost" size="icon" className="text-slate-500 hover:text-indigo-600">
                    <Settings className="w-5 h-5" />
                </Button>
            </DialogTrigger>
            <DialogContent className="sm:max-w-[600px] bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800">
                <DialogHeader>
                    <DialogTitle className="text-xl font-semibold flex items-center gap-2">
                        <Settings className="w-5 h-5 text-indigo-500" />
                        AI 配置
                    </DialogTitle>
                    <DialogDescription>
                        配置您的 AI 模型服务 (配置保存于浏览器本地)
                    </DialogDescription>
                </DialogHeader>

                <Tabs defaultValue="deepseek" className="w-full mt-4">
                    <TabsList className="grid w-full grid-cols-4 mb-4">
                        <TabsTrigger value="deepseek">DeepSeek</TabsTrigger>
                        <TabsTrigger value="openai">OpenAI</TabsTrigger>
                        <TabsTrigger value="claude">Claude</TabsTrigger>
                        <TabsTrigger value="gemini">Gemini</TabsTrigger>
                    </TabsList>

                    {(['deepseek', 'openai', 'claude', 'gemini'] as AIProvider[]).map((provider) => (
                        <TabsContent key={provider} value={provider} className="space-y-4 animate-in fade-in slide-in-from-left-2 duration-300">
                            <div className="flex items-center justify-between p-4 rounded-lg border bg-slate-50 dark:bg-slate-800/50">
                                <div className="space-y-0.5">
                                    <Label className="text-base font-medium">启用 {provider.charAt(0).toUpperCase() + provider.slice(1)}</Label>
                                    <p className="text-xs text-muted-foreground">允许使用此服务商进行对话</p>
                                </div>
                                <Switch
                                    checked={config[provider].enabled}
                                    onCheckedChange={(checked) => updateProvider(provider, { enabled: checked })}
                                />
                            </div>

                            <div className="grid gap-4 py-2">
                                <div className="grid gap-2">
                                    <Label>API 密钥</Label>
                                    <Input
                                        type="password"
                                        placeholder={`输入 ${provider} API Key`}
                                        value={config[provider].apiKey}
                                        onChange={(e) => updateProvider(provider, { apiKey: e.target.value })}
                                        className="font-mono bg-white dark:bg-slate-950"
                                    />
                                </div>
                                <div className="grid grid-cols-2 gap-4">
                                    <div className="grid gap-2">
                                        <Label>接口地址 (Base URL)</Label>
                                        <Input
                                            placeholder="https://api.example.com/v1"
                                            value={config[provider].baseUrl}
                                            onChange={(e) => updateProvider(provider, { baseUrl: e.target.value })}
                                            className="font-mono bg-white dark:bg-slate-950"
                                        />
                                    </div>
                                    <div className="grid gap-2">
                                        <Label>模型名称</Label>
                                        <Input
                                            placeholder="gpt-4, claude-3, etc."
                                            value={config[provider].model}
                                            onChange={(e) => updateProvider(provider, { model: e.target.value })}
                                            className="font-mono bg-white dark:bg-slate-950"
                                        />
                                    </div>
                                </div>
                            </div>
                        </TabsContent>
                    ))}
                </Tabs>

                <DialogFooter className="mt-6 gap-2">
                    <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
                    <Button onClick={handleSave} className="bg-indigo-600 hover:bg-indigo-700 text-white gap-2">
                        <Save className="w-4 h-4" />
                        保存设置
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    )
}
