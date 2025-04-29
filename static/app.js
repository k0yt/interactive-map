let currentUser = null;

const map = L.map('map').setView([20, 0], 2);
L.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png', {
  attribution: '&copy; OpenStreetMap contributors'
}).addTo(map);

fetch('countries.geojson')
  .then(r => r.json())
  .then(data => {
    L.geoJSON(data, {
      style: () => ({ color: 'gray', weight: 1, fillOpacity: 0 }),
      onEachFeature: (feature, layer) => {
        feature.id = feature.properties["ISO3166-1-Alpha-3"];
        layer.on('click', () => onAreaClick(feature));
      }
    }).addTo(map);
    refreshAreas();
  });

document.getElementById('setName').onclick = () => {
  const input = document.getElementById('username');
  const err   = document.getElementById('inputError');
  const name  = input.value.trim();
  if (!name) {
    err.textContent = 'Пожалуйста, введите имя';
    return;
  }
  err.textContent = '';
  currentUser = name;
  input.disabled = true;
  document.getElementById('setName').disabled = true;
};

function onAreaClick(feature) {
  // Показываем секцию выбранной страны
  document.getElementById('selection').classList.remove('hidden');
  document.getElementById('countryName').textContent = feature.properties["name"];
  const areaId = feature.id;

  if (currentUser) {
    // Авторизованный пользователь — сохраняем отметку
    fetch('/api/mark', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user: currentUser, area_id: areaId })
    }).then(() => {
      refreshAreas();
      showUsers(areaId);
    });
  } else {
    // Анонимный просмотр — просто показываем список
    showUsers(areaId);
  }
}

function refreshAreas() {
  fetch('/api/areas')
    .then(r => r.json())
    .then(list => {
      list.forEach(a => {
        map.eachLayer(layer => {
          if (layer.feature && layer.feature.id === a.id) {
            layer.setStyle({
              fillColor: a.count ? getColor(a.count) : 'transparent',
              fillOpacity: a.count ? 0.6 : 0
            });
          }
        });
      });
    });
}

function getColor(c) {
  if (c < 3)  return '#fff7bc';
  if (c < 6)  return '#fee391';
  if (c < 10) return '#fec44f';
  return '#fe9929';
}

function showUsers(areaId) {
  fetch(`/api/users?area_id=${areaId}`)
    .then(r => r.json())
    .then(data => {
      const ul = document.getElementById('usersList');
      ul.innerHTML = '';
      if (data.length === 0) {
        const li = document.createElement('li');
        li.textContent = '(ещё никто не отметил эту страну)';
        ul.appendChild(li);
        return;
      }
      data.forEach(n => {
        const li = document.createElement('li');
        li.textContent = n;
        ul.appendChild(li);
      });
    });
}
