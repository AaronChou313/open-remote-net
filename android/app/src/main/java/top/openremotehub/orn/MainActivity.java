package top.openremotehub.orn;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.*;
import android.view.*;
import android.widget.*;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.*;
import top.openremotehub.orn.core.mobile.Mobile;

/** Native phone/tablet management UI, backed by the desktop's identical LocalAPI. */
public final class MainActivity extends Activity {
    private final ExecutorService worker = Executors.newFixedThreadPool(2);
    private final Handler main = new Handler(Looper.getMainLooper());
    private LinearLayout page, binding;
    private TextView status, message;
    private GridLayout devices;
    private boolean busy, refreshing, active;
    private String previous = "";
    private String lastFailure = "";
    private final Runnable poll = new Runnable() {
        @Override public void run() { if (active) { refresh(); main.postDelayed(this,5000); } }
    };
    interface Work { void run() throws Exception; }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS},1);
        startForegroundService(new Intent(this,OrnService.class));
        build();
    }
    private int dp(int value) { return Math.round(value*getResources().getDisplayMetrics().density); }
    private int color(int resource) { return getColor(resource); }
    private TextView text(String value,int size,boolean bold) {
        TextView view = new TextView(this);
        view.setText(value); view.setTextSize(size); view.setTextColor(color(R.color.text));
        if (bold) view.setTypeface(null,Typeface.BOLD);
        view.setPadding(0,dp(4),0,dp(4));
        return view;
    }
    private LinearLayout column() {
        LinearLayout layout = new LinearLayout(this); layout.setOrientation(LinearLayout.VERTICAL); return layout;
    }
    private LinearLayout card() {
        LinearLayout layout = column(); layout.setPadding(dp(20),dp(16),dp(20),dp(16));
        GradientDrawable background = new GradientDrawable();
        background.setColor(color(R.color.card)); background.setCornerRadius(dp(16));
        background.setStroke(dp(1),color(R.color.outline)); layout.setBackground(background);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1,-2); params.bottomMargin = dp(16);
        layout.setLayoutParams(params); return layout;
    }
    private Button button(String label,Work work) {
        Button button = new Button(this); button.setText(label); button.setAllCaps(false);
        button.setOnClickListener(view -> perform(button,work)); return button;
    }
    private void build() {
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(true);
        scroll.setBackgroundColor(color(R.color.surface));
        page = column(); page.setPadding(dp(20),dp(24),dp(20),dp(24)); scroll.addView(page);
        page.addView(text("OpenRemoteHub",14,true)); page.addView(text("设备网络",30,true));
        LinearLayout self = card(); status = text("正在启动 ORN…",17,true); self.addView(status);
        LinearLayout controls = new LinearLayout(this);
        controls.addView(button("重新连接",() -> {Mobile.start(new File(getFilesDir(),"orn").getAbsolutePath(),Build.MANUFACTURER+" "+Build.MODEL); api("/control",new JSONObject().put("action","reconnect"),"POST"); onUI(() -> info("已请求重新连接"));}));
        controls.addView(button("刷新",() -> {}));
        self.addView(controls); page.addView(self);
        message = text("",14,false); message.setTextColor(color(R.color.muted)); page.addView(message);
        binding = card(); page.addView(binding);
        devices = new GridLayout(this); page.addView(devices);
        LinearLayout options = new LinearLayout(this);
        options.addView(button("网络诊断",() -> {
            Object result=api("/netcheck",new JSONObject(),"POST");
            onUI(() -> {if (active) new AlertDialog.Builder(this).setTitle("网络诊断").setMessage(result.toString()).setPositiveButton("关闭",null).show();});
        }));
        Button leave = new Button(this); leave.setText("退出网络"); leave.setAllCaps(false);
        leave.setOnClickListener(view -> new AlertDialog.Builder(this).setTitle("退出当前网络？")
            .setMessage("移除此设备的账号绑定和网络身份。重新连接需要绑定账号。")
            .setNegativeButton("取消",null).setPositiveButton("退出",(dialog,which) -> perform(leave,() -> api("/leave",new JSONObject().put("confirm",true),"POST"))).show());
        options.addView(leave); page.addView(options);
        setContentView(scroll);
        // Keep controls clear of status/navigation bars under Android 15 edge-to-edge.
        scroll.setOnApplyWindowInsetsListener((view,insets) -> {
            if (Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets bars=insets.getInsets(WindowInsets.Type.systemBars());
                view.setPadding(bars.left,bars.top,bars.right,bars.bottom);
            } else {
                view.setPadding(insets.getSystemWindowInsetLeft(),insets.getSystemWindowInsetTop(),insets.getSystemWindowInsetRight(),insets.getSystemWindowInsetBottom());
            }
            return insets;
        });
    }
    @Override public void onResume() { super.onResume(); active=true; onUI(poll); }
    @Override public void onPause() { active=false; main.removeCallbacks(poll); super.onPause(); }
    @Override public void onDestroy() { worker.shutdownNow(); super.onDestroy(); }
    private Object api(String path,JSONObject body,String method) throws Exception {
        String token=Mobile.token();
        if (token.isEmpty()) throw new IOException(OrnService.error.isEmpty()?"ORN 正在启动，请稍候":OrnService.error);
        HttpURLConnection connection=(HttpURLConnection)new URL("http://127.0.0.1:4789/api/v1"+path).openConnection();
        connection.setConnectTimeout(5000); connection.setReadTimeout(18000); connection.setRequestMethod(method);
        connection.setRequestProperty("Authorization","Bearer "+token);
        try {
            if (body!=null) {
                connection.setDoOutput(true); connection.setRequestProperty("Content-Type","application/json");
                try (OutputStream output=connection.getOutputStream()) { output.write(body.toString().getBytes(StandardCharsets.UTF_8)); }
            }
            int code=connection.getResponseCode();
            InputStream stream=code>=400?connection.getErrorStream():connection.getInputStream();
            String content;
            try (BufferedReader input=new BufferedReader(new InputStreamReader(stream,StandardCharsets.UTF_8))) {
                StringBuilder result=new StringBuilder(); String line;
                while ((line=input.readLine())!=null) result.append(line);
                content=result.toString();
            }
            Object result=new JSONTokener(content).nextValue();
            if (code>=400) throw new IOException(result instanceof JSONObject?((JSONObject)result).optString("detail","操作未完成"):"操作未完成");
            return result;
        } finally { connection.disconnect(); }
    }
    private void perform(Button button,Work work) {
        if (busy) return; busy=true; button.setEnabled(false);
        worker.execute(() -> {
            try { work.run(); }
            catch (Exception error) { onUI(() -> info(error.getMessage())); }
            finally { onUI(() -> { busy=false; button.setEnabled(true); previous=""; refresh(); }); }
        });
    }
    private void onUI(Runnable work) {
        main.post(() -> {if (!isDestroyed()&&!isFinishing()) work.run();});
    }
    private void info(String value) { message.setText(value); }
    private void refresh() {
        if (busy||refreshing||!active) return;
        refreshing=true;
        worker.execute(() -> {
            try {
                JSONObject self=(JSONObject)api("/status",null,"GET");
                JSONArray peers=(JSONArray)api("/peers",null,"GET");
                String snapshot=self.toString()+peers.toString();
                onUI(() -> {
                    if (busy) return;
                    if (!lastFailure.isEmpty()) { info(""); lastFailure=""; }
                    if (!snapshot.equals(previous)) { render(self,peers); previous=snapshot; }
                });
            } catch (Exception error) { onUI(() -> {lastFailure=error.getMessage(); info(lastFailure);}); }
            finally { onUI(() -> refreshing=false); }
        });
    }
    private String ips(JSONObject peer) {
        JSONArray addresses=peer.optJSONArray("ips");
        return addresses!=null&&addresses.length()>0?addresses.optString(0):"等待分配地址";
    }
    private void render(JSONObject self,JSONArray peers) {
        JSONObject device=self.optJSONObject("device");
        status.setText((device==null?"此设备":device.optString("hostname"))+"\n"+(device==null?"":ips(device))+" · "+("Running".equals(self.optString("backend_state"))?"已连接":self.optString("backend_state")));
        JSONObject cloud=self.optJSONObject("cloud");
        binding.removeAllViews();
        boolean bound=cloud!=null&&cloud.optBoolean("bound"); binding.setVisibility(bound?View.GONE:View.VISIBLE);
        if (!bound) {
            binding.addView(text("绑定 Cloud 账号",18,true));
            if (cloud!=null&&cloud.has("pending")) {
                JSONObject pending=cloud.optJSONObject("pending");
                binding.addView(text(pending.optString("user_code"),24,true));
                binding.addView(button("前往 Cloud 确认绑定",() -> onUI(() -> open(pending.optString("verification_url")))));
            } else {
                binding.addView(button("绑定账号",() -> api("/cloud/bind",new JSONObject().put("cloud_url","https://cloud.openremotehub.top"),"POST")));
            }
        }
        if (cloud!=null&&!cloud.optString("error").isEmpty()) info(cloud.optString("error"));
        devices.removeAllViews();
        int columns=getResources().getConfiguration().screenWidthDp>=720?2:1;
        devices.setColumnCount(columns);
        for (int index=0;index<peers.length();index++) {
            JSONObject peer=peers.optJSONObject(index); if (peer==null) continue;
            LinearLayout entry=card();
            GridLayout.LayoutParams params=new GridLayout.LayoutParams();
            params.width=0; params.columnSpec=GridLayout.spec(GridLayout.UNDEFINED,1f);
            params.setMargins(dp(4),0,dp(4),dp(14)); entry.setLayoutParams(params);
            entry.addView(text(peer.optString("hostname"),20,true));
            TextView detail=text(ips(peer)+" · "+(peer.optBoolean("online")?"在线":"离线"),14,false);
            detail.setTextColor(color(R.color.muted)); entry.addView(detail);
            String path=peer.optString("path");
            entry.addView(text("direct".equals(path)?"直连":"relay".equals(path)?"中继":"待连接",14,true));
            String ip=ips(peer);
            LinearLayout actions=new LinearLayout(this);
            actions.addView(button("检测",() -> {
                JSONObject result=(JSONObject)api("/ping",new JSONObject().put("peer_ip",ip),"POST");
                onUI(() -> info(result.optString("Err").isEmpty()?peer.optString("hostname")+" · "+Math.round(result.optDouble("LatencySeconds")*1000)+" ms":result.optString("Err")));
            }));
            actions.addView(button("控制台",() -> {
                JSONObject result=(JSONObject)api("/console",new JSONObject().put("peer_ip",ip),"POST");
                onUI(() -> open(result.optString("url")));
            }));
            entry.addView(actions);
            Button desktop=button("远程桌面",() -> {
                JSONObject result=(JSONObject)api("/forwards",new JSONObject().put("peer_ip",ip).put("remote_port",21118),"POST");
                onUI(() -> desktop(result.optString("address")));
            });
            desktop.setEnabled(peer.optBoolean("online")); entry.addView(desktop); devices.addView(entry);
        }
        if (peers.length()==0) devices.addView(text("将其他设备绑定到同一账号后显示在这里。",15,false));
    }
    private void open(String url) {
        try { startActivity(new Intent(Intent.ACTION_VIEW,Uri.parse(url))); }
        catch (ActivityNotFoundException error) { info("此设备没有可用的浏览器"); }
    }
    static Uri rustDeskURI(String address) {
        // RustDesk reads Dart Uri.path directly, so keep the port colon literal.
        return Uri.parse("rustdesk://connection/new/"+address);
    }
    private void desktop(String address) {
        Uri uri=rustDeskURI(address);
        try { startActivity(new Intent(Intent.ACTION_VIEW,uri)); }
        catch (ActivityNotFoundException error) {
            getSystemService(ClipboardManager.class).setPrimaryClip(ClipData.newPlainText("ORN 连接地址",address));
            new AlertDialog.Builder(this).setTitle("打开 RustDesk").setMessage("请安装 RustDesk。连接地址已复制："+address).setPositiveButton("关闭",null).show();
        }
    }
}
