package top.openremotehub.orn;

import android.app.*;
import android.content.*;
import android.net.*;
import android.os.*;
import java.io.File;
import org.json.*;
import java.util.*;
import java.util.concurrent.*;
import top.openremotehub.orn.core.mobile.Mobile;

/** Owns the Go runtime while the Activity or RustDesk is in the foreground. */
public final class OrnService extends Service {
    static volatile String error = "";
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private ConnectivityManager connectivity;
    private ConnectivityManager.NetworkCallback callback;
    private PowerManager.WakeLock connectionWakeLock;
    private long desktopStartingUntil;
    private boolean destroyed;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Runnable maintainConnection = this::checkConnections;
    private void checkConnections() {
        worker.execute(() -> {
            boolean connected=Mobile.activeForwards()>0;
            main.post(() -> {
                if (destroyed) return;
                if (connected || SystemClock.elapsedRealtime()<desktopStartingUntil) {
                    connectionWakeLock.acquire(90000);
                } else if (connectionWakeLock.isHeld()) {
                    connectionWakeLock.release();
                }
                main.postDelayed(maintainConnection,15000);
            });
        });
    }
    private final Runnable rebind = () -> worker.execute(() -> {
        try { Mobile.updateNetwork(networkInfo()); Mobile.rebind(); } catch (Exception e) { error = "网络切换后正在重连"; }
    });

    @Override public void onCreate() {
        super.onCreate();
        go.Seq.setContext(getApplicationContext());
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel("orn", "ORN 连接", NotificationManager.IMPORTANCE_LOW));
        PendingIntent open = PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_IMMUTABLE);
        Notification notification = new Notification.Builder(this,"orn")
            .setSmallIcon(R.drawable.ic_orn).setContentTitle("OpenRemoteHub · ORN")
            .setContentText("正在启动设备网络").setContentIntent(open).setOngoing(true).build();
        startForeground(1,notification);
        connectionWakeLock=getSystemService(PowerManager.class).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK,"OpenRemoteHub:Forwarding");
        connectionWakeLock.setReferenceCounted(false);
        connectivity = getSystemService(ConnectivityManager.class);
        callback = new ConnectivityManager.NetworkCallback() {
            @Override public void onAvailable(Network network) { scheduleRebind(); }
            @Override public void onLost(Network network) { scheduleRebind(); }
            @Override public void onLinkPropertiesChanged(Network network,LinkProperties properties) { scheduleRebind(); }
        };
        connectivity.registerDefaultNetworkCallback(callback);
    }
    private String networkInfo() throws JSONException {
        JSONObject result=new JSONObject();
        JSONArray interfaces=new JSONArray();
        Set<String> names=new HashSet<>();
        Network active=connectivity.getActiveNetwork();
        for (Network network:connectivity.getAllNetworks()) {
            LinkProperties link=connectivity.getLinkProperties(network);
            if (link==null || link.getInterfaceName()==null) continue;
            String name=link.getInterfaceName();
            if (network.equals(active)) {
                result.put("default_interface",name);
                for (RouteInfo route:link.getRoutes()) {
                    if (route.isDefaultRoute() && route.getGateway()!=null) result.put("gateway",route.getGateway().getHostAddress());
                }
            }
            if (!names.add(name)) continue;
            JSONArray addresses=new JSONArray();
            for (LinkAddress address:link.getLinkAddresses()) {
                String ip=address.getAddress().getHostAddress().split("%",2)[0];
                addresses.put(ip+"/"+address.getPrefixLength());
            }
            int mtu=Build.VERSION.SDK_INT>=29?link.getMtu():1500;
            interfaces.put(new JSONObject().put("name",name).put("mtu",mtu>0?mtu:1500).put("addresses",addresses));
        }
        result.put("interfaces",interfaces);
        return result.toString();
    }
    private void scheduleRebind() {
        main.removeCallbacks(rebind);
        main.postDelayed(rebind,1500);
    }
    @Override public int onStartCommand(Intent intent,int flags,int id) {
        if (intent!=null && intent.getBooleanExtra("desktop",false)) {
            desktopStartingUntil=SystemClock.elapsedRealtime()+90000;
            connectionWakeLock.acquire(90000);
        }
        worker.execute(this::startRuntime);
        return START_STICKY;
    }
    private void startRuntime() {
        try {
            Mobile.updateNetwork(networkInfo());
            Mobile.start(new File(getFilesDir(),"orn").getAbsolutePath(),Build.MANUFACTURER+" "+Build.MODEL);
            error="";
            updateNotification("ORN 正在运行，点击管理设备");
            main.post(() -> {
                if (destroyed) return;
                main.removeCallbacks(maintainConnection);
                main.post(maintainConnection);
            });
        } catch (Exception e) {
            error="ORN 启动失败："+e.getMessage();
            android.util.Log.e("ORN",error);
            updateNotification("启动失败，点击重试");
        }
    }
    private void updateNotification(String status) {
        PendingIntent open=PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_IMMUTABLE);
        getSystemService(NotificationManager.class).notify(1,new Notification.Builder(this,"orn")
            .setSmallIcon(R.drawable.ic_orn).setContentTitle("OpenRemoteHub · ORN")
            .setContentText(status).setContentIntent(open).setOngoing(true).build());
    }
    @Override public IBinder onBind(Intent intent) { return null; }
    @Override public void onDestroy() {
        destroyed=true;
        connectivity.unregisterNetworkCallback(callback);
        main.removeCallbacks(rebind);
        main.removeCallbacks(maintainConnection);
        if (connectionWakeLock.isHeld()) connectionWakeLock.release();
        worker.execute(() -> { try { Mobile.stop(); } catch (Exception ignored) {} });
        worker.shutdown();
        super.onDestroy();
    }
}
